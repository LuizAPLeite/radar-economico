"""Job do IBGE: lê os municípios e os centroides do raw e grava staging e mart."""

import logging
from dataclasses import dataclass
from datetime import datetime

import psycopg

from radar_pipeline import runs
from radar_pipeline.ibge import extract, load, transform

log = logging.getLogger(__name__)

# Nome do job no CLI e em mart.pipeline_runs.
NAME = "ibge"

# O Brasil tem 5.571 municípios: bem menos que isso indica uma resposta
# truncada ou um formato novo, e não deve substituir a tabela que já existe.
MIN_MUNICIPALITIES = 5_000

# Hoje só 1 município (criado depois da malha de centroides do IBGE) fica sem
# coordenadas. Muitos mais sugerem mudança no arquivo de centroides: vira aviso.
MAX_WITHOUT_COORDINATES = 10


@dataclass(frozen=True)
class Summary:
    source_run_id: int  # a coleta mais recente entre os dois conjuntos
    source_collected_at: datetime
    municipalities: int
    with_coordinates: int
    changed_in_mart: int  # inseridos ou alterados (quase sempre 0)
    warnings: tuple[str, ...] = ()

    @property
    def rows_written(self) -> int:
        return self.changed_in_mart


def run(conn: psycopg.Connection, *, full_refresh: bool = False) -> Summary | None:
    """Grava os municípios, se o IBGE mudou desde a última carga (ou com full_refresh)."""
    newest = extract.newest_run(conn)
    if newest is None:
        log.warning("nenhuma resposta do IBGE no raw; nada a processar")
        return None
    if not full_refresh and newest <= (runs.last_source_run(conn, NAME) or 0):
        log.info("nada novo do IBGE desde a coleta %d; nada a processar", newest)
        return None

    payloads = extract.latest_payloads(conn)
    missing = {"municipios", "centroides"} - payloads.keys()
    if missing:
        log.warning("raw do IBGE incompleto (faltam: %s); nada a processar", sorted(missing))
        return None

    municipios, centroides = payloads["municipios"], payloads["centroides"]
    frame = transform.to_municipalities(municipios.data, centroides.data)
    if frame.height < MIN_MUNICIPALITIES:
        raise ValueError(
            f"só {frame.height} municípios no raw do IBGE (esperado ≥ {MIN_MUNICIPALITIES})"
        )

    with_coordinates = frame.drop_nulls(["latitude", "longitude"]).height
    warnings = []
    if (missing := frame.height - with_coordinates) > MAX_WITHOUT_COORDINATES:
        warnings.append(f"{missing} municípios sem coordenadas no IBGE: ficam fora do mapa")
    for warning in warnings:
        log.warning("aviso: %s", warning)

    changed = load.replace(conn, frame)

    summary = Summary(
        source_run_id=max(municipios.run_id, centroides.run_id),
        source_collected_at=max(municipios.fetched_at, centroides.fetched_at),
        municipalities=frame.height,
        with_coordinates=with_coordinates,
        changed_in_mart=changed,
        warnings=tuple(warnings),
    )
    log.info(
        "municípios: %d, com coordenadas: %d, alterados na mart: %d",
        summary.municipalities,
        summary.with_coordinates,
        summary.changed_in_mart,
    )
    return summary
