package assistant

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/LuizAPLeite/radar-economico/api/internal/cache"
)

// Limites da pergunta, em caracteres.
const (
	MinQuestionLength = 3
	MaxQuestionLength = 300
)

// AnswerTTL é por quanto tempo uma resposta fica guardada: a mart muda no
// máximo uma vez por dia, e cada pergunta repetida economiza a cota do LLM.
const AnswerTTL = 24 * time.Hour

var (
	// ErrBadQuestion: pergunta vazia, curta ou longa demais.
	ErrBadQuestion = errors.New("pergunta inválida")
	// ErrDailyLimit: o limite de perguntas novas do dia (ASSISTANT_DAILY_LIMIT) acabou.
	ErrDailyLimit = errors.New("limite diário de perguntas atingido")
	// ErrBadPlan: o LLM devolveu um SQL recusado ou que falhou no banco.
	ErrBadPlan = errors.New("o assistente não conseguiu montar uma consulta válida")
)

// Answer é a resposta a uma pergunta, pronta para virar JSON.
type Answer struct {
	Question    string    `json:"question"`
	Answerable  bool      `json:"answerable"`
	Title       string    `json:"title"`
	Explanation string    `json:"explanation"`
	SQL         string    `json:"sql"`
	Chart       ChartSpec `json:"chart"`
	Columns     []Column  `json:"columns"`
	Rows        [][]any   `json:"rows"`
	Truncated   bool      `json:"truncated"`
	Cached      bool      `json:"cached"`
}

// Service responde perguntas: confere a pergunta, procura no cache, respeita
// o limite diário, pede o plano ao LLM, confere o SQL (CheckSQL) e executa.
type Service struct {
	planner    Planner
	executor   Executor
	logger     *slog.Logger
	dailyLimit int
	answers    *cache.Cache[Answer]

	// Relógio e fuso do limite diário (o dia vira à meia-noite de Brasília).
	now func() time.Time
	loc *time.Location

	mu    sync.Mutex
	day   string // dia (AAAA-MM-DD) a que `count` se refere
	count int    // perguntas que chegaram ao LLM nesse dia
}

func NewService(p Planner, e Executor, logger *slog.Logger, dailyLimit int, loc *time.Location) *Service {
	return &Service{
		planner:    p,
		executor:   e,
		logger:     logger,
		dailyLimit: dailyLimit,
		answers:    cache.New[Answer](AnswerTTL),
		now:        time.Now,
		loc:        loc,
	}
}

// Ask responde uma pergunta. Os erros possíveis embrulham ErrBadQuestion,
// ErrDailyLimit, ErrQuotaExceeded ou ErrBadPlan; outros são falhas do
// provedor ou do banco.
func (s *Service) Ask(ctx context.Context, question string) (Answer, error) {
	question = strings.TrimSpace(question)
	if n := utf8.RuneCountInString(question); n < MinQuestionLength || n > MaxQuestionLength {
		return Answer{}, fmt.Errorf("%w: use de %d a %d caracteres", ErrBadQuestion, MinQuestionLength, MaxQuestionLength)
	}

	key := normalize(question)
	if a, ok := s.answers.Get(key); ok {
		a.Question, a.Cached = question, true
		return a, nil
	}
	if !s.take() {
		return Answer{}, ErrDailyLimit
	}

	plan, err := s.planner.Plan(ctx, question)
	if err != nil {
		return Answer{}, err
	}
	answer := Answer{
		Question: question, Answerable: plan.Answerable, Title: plan.Title,
		Explanation: plan.Explanation, Chart: ChartSpec{Type: "none", Y: []string{}},
		Columns: []Column{}, Rows: [][]any{},
	}
	if plan.Answerable {
		sql, err := CheckSQL(plan.SQL)
		if err != nil {
			s.logger.Warn("SQL do assistente recusado", "question", question, "sql", plan.SQL, "err", err)
			return Answer{}, fmt.Errorf("%w: %w", ErrBadPlan, err)
		}
		result, err := s.executor.Run(ctx, sql)
		if err != nil {
			// Ex.: coluna que não existe, permissão negada, tempo esgotado.
			s.logger.Warn("SQL do assistente falhou", "question", question, "sql", sql, "err", err)
			return Answer{}, fmt.Errorf("%w: %w", ErrBadPlan, err)
		}
		answer.SQL = sql
		answer.Columns, answer.Rows, answer.Truncated = result.Columns, result.Rows, result.Truncated
		answer.Chart = checkChart(plan.Chart, result.Columns, len(result.Rows))
	}
	s.logger.Info("pergunta respondida", "question", question, "answerable", answer.Answerable,
		"rows", len(answer.Rows), "chart", answer.Chart.Type)

	s.answers.Set(key, answer)
	return answer, nil
}

// take reserva uma pergunta do limite do dia. Conta antes de chamar o LLM:
// mesmo uma chamada que falha gasta a cota do provedor.
func (s *Service) take() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	today := s.now().In(s.loc).Format(time.DateOnly)
	if today != s.day {
		s.day, s.count = today, 0
	}
	if s.count >= s.dailyLimit {
		return false
	}
	s.count++
	return true
}

// normalize faz perguntas iguais darem a mesma chave no cache:
// "Qual a Selic hoje?" e "qual a  selic hoje" são a mesma pergunta.
func normalize(q string) string {
	q = strings.ToLower(strings.Join(strings.Fields(q), " "))
	return strings.TrimRight(q, "?!. ")
}

// checkChart confere o gráfico que o LLM sugeriu contra as colunas que a
// consulta devolveu de verdade. Sugestão incoerente vira "none": a tabela
// sempre aparece, então nada se perde.
func checkChart(c ChartSpec, cols []Column, rows int) ChartSpec {
	none := ChartSpec{Type: "none", Y: []string{}}
	if rows == 0 || (c.Type != "line" && c.Type != "bar") || len(c.Y) == 0 || len(c.Y) > 3 {
		return none
	}
	typeOf := func(name string) string {
		i := slices.IndexFunc(cols, func(col Column) bool { return col.Name == name })
		if i < 0 {
			return ""
		}
		return cols[i].Type
	}
	x := typeOf(c.X)
	if x == "" || (c.Type == "line" && x != "date") || (c.Type == "bar" && rows > 30) {
		return none
	}
	for _, y := range c.Y {
		if y == c.X || typeOf(y) != "number" {
			return none
		}
	}
	return c
}
