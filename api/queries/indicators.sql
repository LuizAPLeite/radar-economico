-- name: ListIndicators :many
-- Todos os indicadores, cada um com o seu valor mais recente.
-- sqlc.embed(i) faz o resultado trazer o struct MartIndicator inteiro.
-- O LATERAL roda a subconsulta uma vez por indicador; com ORDER BY ... LIMIT 1
-- ela usa o índice da chave primária e lê uma única linha.
SELECT sqlc.embed(i),
       latest.ref_date AS last_date,
       latest.value    AS last_value
  FROM mart.indicators AS i
  JOIN LATERAL (
        SELECT v.ref_date, v.value
          FROM mart.indicator_values AS v
         WHERE v.indicator_id = i.id
         ORDER BY v.ref_date DESC
         LIMIT 1
       ) AS latest ON true
 ORDER BY i.id;

-- name: GetIndicator :one
-- O sqlc expande o * para a lista de colunas na hora de gerar o código:
-- se o schema mudar, o código gerado muda junto.
SELECT *
  FROM mart.indicators
 WHERE id = $1;

-- name: ListIndicatorValues :many
-- Série de um indicador num intervalo fechado de datas, em ordem cronológica.
SELECT ref_date, value
  FROM mart.indicator_values
 WHERE indicator_id = sqlc.arg(indicator_id)
   AND ref_date >= sqlc.arg(from_date)
   AND ref_date <= sqlc.arg(to_date)
 ORDER BY ref_date;
