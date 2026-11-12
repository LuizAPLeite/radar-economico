package db_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/LuizAPLeite/radar-economico/api/internal/db"
)

func d(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }

// Teste de integração das queries de combustíveis, com dados conhecidos no
// lugar dos reais (só dentro da transação, que sofre rollback no fim).
func TestFuelQueries(t *testing.T) {
	ctx, tx := beginTestTx(t)
	mustExec(t, ctx, tx, deleteFuelData...)
	mustExec(t, ctx, tx,
		`INSERT INTO mart.fuel_products (id, name, unit, updated_at) VALUES
		   ('gasolina', 'Gasolina comum', 'R$/l', '2026-11-07 19:00:00+00'),
		   ('gnv', 'Gás natural veicular', 'R$/m³', '2026-11-07 19:00:00+00')`,
		`INSERT INTO mart.fuel_prices_weekly (product_id, scope, week_start, avg_price, n_stations) VALUES
		   ('gasolina', 'BR', '2026-10-19', 6.512, 4600),
		   ('gasolina', 'BR', '2026-10-26', 6.505, 4560),
		   ('gasolina', 'SP', '2026-10-26', 6.290, 900),
		   ('gnv',      'BR', '2026-10-26', 4.890, 369)`,
		`INSERT INTO mart.municipalities (ibge_code, name, uf, latitude, longitude) VALUES
		   (3550308, 'São Paulo', 'SP', -23.6501, -46.6481),
		   (2903201, 'Barreiras', 'BA', -12.1439, -44.9968)
		 ON CONFLICT (ibge_code) DO UPDATE SET
		   latitude = EXCLUDED.latitude, longitude = EXCLUDED.longitude`, // sobrepõe os reais, se houver
		`INSERT INTO mart.fuel_prices_municipal
		   (product_id, month, ibge_code, avg_price, min_price, max_price, n_stations) VALUES
		   ('gasolina', '2026-09-01', 3550308, 6.30, 5.99, 6.99, 120),
		   ('gasolina', '2026-10-01', 3550308, 6.29, 5.89, 6.89, 118),
		   ('gasolina', '2026-10-01', 2903201, 6.89, 6.79, 7.09, 9)`,
	)
	q := db.New(tx)

	t.Run("ListFuelProducts traz a média nacional mais recente", func(t *testing.T) {
		rows, err := q.ListFuelProducts(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("len = %d, quer 2", len(rows))
		}
		gas := rows[0] // ordenado por id
		if gas.MartFuelProduct.ID != "gasolina" || gas.LastAvgPrice != 6.505 ||
			gas.LastNStations != 4560 || !gas.LastWeekStart.Equal(d(2026, 10, 26)) {
			t.Errorf("gasolina = %+v", gas)
		}
	})

	t.Run("GetFuelProduct inexistente devolve pgx.ErrNoRows", func(t *testing.T) {
		if _, err := q.GetFuelProduct(ctx, "querosene"); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("err = %v, quer pgx.ErrNoRows", err)
		}
	})

	t.Run("ListFuelPricesWeekly filtra escopo e intervalo", func(t *testing.T) {
		rows, err := q.ListFuelPricesWeekly(ctx, db.ListFuelPricesWeeklyParams{
			ProductID: "gasolina", Scope: "BR", FromDate: d(2026, 10, 20), ToDate: d(2026, 12, 31),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || !rows[0].WeekStart.Equal(d(2026, 10, 26)) || rows[0].AvgPrice != 6.505 {
			t.Fatalf("rows = %+v", rows)
		}
	})

	t.Run("ListFuelMonths lista os meses em ordem", func(t *testing.T) {
		months, err := q.ListFuelMonths(ctx, "gasolina")
		if err != nil {
			t.Fatal(err)
		}
		if len(months) != 2 || !months[0].Equal(d(2026, 9, 1)) || !months[1].Equal(d(2026, 10, 1)) {
			t.Fatalf("meses = %v", months)
		}
	})

	t.Run("ListFuelPricesMunicipal junta as coordenadas", func(t *testing.T) {
		rows, err := q.ListFuelPricesMunicipal(ctx, db.ListFuelPricesMunicipalParams{
			ProductID: "gasolina", Month: d(2026, 10, 1),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 2 {
			t.Fatalf("len = %d, quer 2", len(rows))
		}
		ba := rows[0] // ordenado por UF: BA antes de SP
		if ba.Name != "Barreiras" || ba.Latitude != -12.1439 || ba.MinPrice != 6.79 || ba.NStations != 9 {
			t.Errorf("Barreiras = %+v", ba)
		}
	})
}
