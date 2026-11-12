package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	// Embute o banco de fusos horários no binário: a imagem distroless da
	// API não tem /usr/share/zoneinfo, e o "hoje" daqui é o de Brasília.
	_ "time/tzdata"

	"github.com/LuizAPLeite/radar-economico/api/internal/db"
)

// HealthStore é o que o endpoint de saúde dos dados precisa do banco.
type HealthStore interface {
	ListDatasetFreshness(ctx context.Context) ([]db.ListDatasetFreshnessRow, error)
	ListLastRuns(ctx context.Context) ([]db.ListLastRunsRow, error)
	ListLastSuccessfulRuns(ctx context.Context) ([]db.ListLastSuccessfulRunsRow, error)
}

// dataset é um conjunto de dados que o site mostra, com quantos dias o dado
// mais recente pode ter antes de ser considerado atrasado. Os prazos levam
// em conta fins de semana, feriados e o atraso de publicação de cada fonte.
type dataset struct {
	id, name   string
	maxAgeDays int
}

var datasets = []dataset{
	// Diárias: um feriado emendado com o fim de semana deixa até ~5 dias sem
	// valor novo (do Carnaval na sexta à quarta-feira, por exemplo).
	{"selic", "Meta Selic", 5},
	{"usd_brl", "Dólar comercial (venda)", 5},
	// O IPCA de um mês sai por volta do dia 10 do mês seguinte: logo antes da
	// divulgação, o mais recente tem ~70 dias (setembro, em 9 de novembro).
	{"ipca", "IPCA (variação mensal)", 75},
	// A ANP publica um arquivo por mês, semanas depois do fim dele: a última
	// semana publicada chega a ~40 dias, e a publicação às vezes atrasa.
	{"fuels", "Preços de combustíveis (ANP)", 60},
	// O FRED publica o Brent com cerca de uma semana de atraso (dado da EIA).
	{"brent", "Petróleo Brent", 15},
	// A série semanal oficial da ANP sai na sexta da semana pesquisada; o
	// prazo cobre um atraso de duas semanas na publicação.
	{"fuels_series", "Série semanal oficial da ANP", 21},
}

// now é o relógio do endpoint; os testes trocam por um fixo.
var now = time.Now

var brasilia = func() *time.Location {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		panic(err) // com o tzdata embutido, não acontece
	}
	return loc
}()

// --- Formato da resposta --------------------------------------------------------

type healthReport struct {
	Status    string          `json:"status"` // ok ou degraded
	CheckedAt time.Time       `json:"checked_at"`
	Datasets  []datasetHealth `json:"datasets"`
	Jobs      []jobHealth     `json:"jobs"`
}

type datasetHealth struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Latest     *string `json:"latest"`   // AAAA-MM-DD; null quando não há dados
	AgeDays    *int    `json:"age_days"` // dias desde o dado mais recente (horário de Brasília)
	MaxAgeDays int     `json:"max_age_days"`
	Status     string  `json:"status"` // ok, stale (atrasado) ou missing (sem dados)
}

type jobHealth struct {
	Job           string     `json:"job"`
	LastStatus    string     `json:"last_status"` // success, skipped ou failed
	LastRunAt     time.Time  `json:"last_run_at"`
	LastSuccessAt *time.Time `json:"last_success_at"` // null se nunca deu certo
	CollectedAt   *time.Time `json:"collected_at"`    // quando a fonte da última carga foi coletada
	Warnings      []string   `json:"warnings"`        // avisos de qualidade da última carga
}

// --- Handler --------------------------------------------------------------------

// healthData responde GET /health/data: se os dados estão em dia. Dá 200
// quando está tudo em dia, e 503 quando algum conjunto atrasou ou está vazio,
// ou quando a última execução de algum job falhou. O corpo é o mesmo nos
// dois casos, para quem recebe o alerta ver o que está errado.
//
// A mensagem de erro de um job que falhou nunca sai daqui: ela fica no banco.
func (h *handlers) healthData(w http.ResponseWriter, r *http.Request) {
	latest, err := h.store.ListDatasetFreshness(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	lastRuns, err := h.store.ListLastRuns(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	lastOK, err := h.store.ListLastSuccessfulRuns(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}

	report, err := buildHealthReport(now(), latest, lastRuns, lastOK)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	status := http.StatusOK
	if report.Status != "ok" {
		status = http.StatusServiceUnavailable
	}
	// Quem consulta (um monitor) não deve guardar a resposta.
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, report)
}

// buildHealthReport compara a data mais recente de cada conjunto com o
// prazo dele e junta a situação de cada job. Função pura: os testes passam
// o "agora" e as linhas do banco.
func buildHealthReport(
	at time.Time,
	latest []db.ListDatasetFreshnessRow,
	lastRuns []db.ListLastRunsRow,
	lastOK []db.ListLastSuccessfulRunsRow,
) (healthReport, error) {
	// "Hoje" é o dia em Brasília, à meia-noite UTC, como as datas do banco.
	y, m, d := at.In(brasilia).Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)

	report := healthReport{
		Status:    "ok",
		CheckedAt: at.UTC(),
		Datasets:  make([]datasetHealth, 0, len(datasets)),
		Jobs:      make([]jobHealth, 0, len(lastRuns)),
	}

	latestByID := make(map[string]time.Time, len(latest))
	for _, row := range latest {
		latestByID[row.Dataset] = row.Latest
	}
	for _, ds := range datasets {
		item := datasetHealth{ID: ds.id, Name: ds.name, MaxAgeDays: ds.maxAgeDays, Status: "missing"}
		if date, ok := latestByID[ds.id]; ok {
			shown := date.Format(time.DateOnly)
			age := int(today.Sub(date).Hours() / 24)
			item.Latest, item.AgeDays, item.Status = &shown, &age, "ok"
			if age > ds.maxAgeDays {
				item.Status = "stale"
			}
		}
		if item.Status != "ok" {
			report.Status = "degraded"
		}
		report.Datasets = append(report.Datasets, item)
	}

	okByJob := make(map[string]db.ListLastSuccessfulRunsRow, len(lastOK))
	for _, row := range lastOK {
		okByJob[row.Job] = row
	}
	for _, run := range lastRuns {
		item := jobHealth{Job: run.Job, LastStatus: run.Status, LastRunAt: run.FinishedAt.UTC(), Warnings: []string{}}
		if ok, found := okByJob[run.Job]; found {
			at := ok.FinishedAt.UTC()
			item.LastSuccessAt = &at
			if ok.SourceCollectedAt.Valid {
				collected := ok.SourceCollectedAt.Time.UTC()
				item.CollectedAt = &collected
			}
			if len(ok.Warnings) > 0 { // a coluna é um array JSON ('[]' quando não há avisos)
				if err := json.Unmarshal(ok.Warnings, &item.Warnings); err != nil {
					return healthReport{}, fmt.Errorf("avisos do job %s: %w", run.Job, err)
				}
			}
		}
		if run.Status == "failed" {
			report.Status = "degraded"
		}
		report.Jobs = append(report.Jobs, item)
	}
	return report, nil
}
