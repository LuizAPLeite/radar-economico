-- name: ListFuelProducts :many
-- Combustíveis com a média nacional da semana mais recente de cada um.
SELECT sqlc.embed(p),
       latest.week_start AS last_week_start,
       latest.avg_price  AS last_avg_price,
       latest.n_stations AS last_n_stations
  FROM mart.fuel_products AS p
  JOIN LATERAL (
        SELECT w.week_start, w.avg_price, w.n_stations
          FROM mart.fuel_prices_weekly AS w
         WHERE w.product_id = p.id
           AND w.scope = 'BR'
         ORDER BY w.week_start DESC
         LIMIT 1
       ) AS latest ON true
 ORDER BY p.id;

-- name: GetFuelProduct :one
SELECT *
  FROM mart.fuel_products
 WHERE id = $1;

-- name: ListFuelPricesWeekly :many
-- Série semanal de um combustível no Brasil ('BR') ou numa UF.
SELECT week_start, avg_price, n_stations
  FROM mart.fuel_prices_weekly
 WHERE product_id = sqlc.arg(product_id)
   AND scope = sqlc.arg(scope)
   AND week_start >= sqlc.arg(from_date)
   AND week_start <= sqlc.arg(to_date)
 ORDER BY week_start;

-- name: ListFuelMonths :many
-- Meses com preço por município, para o seletor de mês do mapa.
SELECT DISTINCT month
  FROM mart.fuel_prices_municipal
 WHERE product_id = $1
 ORDER BY month;

-- name: ListFuelPricesMunicipal :many
-- Preço de um combustível em cada município num mês, com as coordenadas
-- para o mapa.
SELECT p.ibge_code, m.name, m.uf, m.latitude, m.longitude,
       p.avg_price, p.min_price, p.max_price, p.n_stations
  FROM mart.fuel_prices_municipal AS p
  JOIN mart.municipalities AS m ON m.ibge_code = p.ibge_code
 WHERE p.product_id = sqlc.arg(product_id)
   AND p.month = sqlc.arg(month)
 ORDER BY m.uf, m.name;
