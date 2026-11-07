"""Job do IBGE: lê os municípios e os centroides do raw e grava staging e mart."""

import logging
from dataclasses import dataclass

import psycopg

from radar_pipeline.ibge import extract, load, transform

log = logging.getLogger(__name__)

# O Brasil tem 5.571 municípios: bem menos que isso indica uma resposta
# truncada ou um formato novo, e não deve substituir a tabela que já existe.
MIN_MUNICIPALITIES = 5_000


@dataclass(frozen=True)
class Summary:
    municipalities: int
    with_coordinates: int


def run(conn: psycopg.Connection) -> Summary | None:
    payloads = extract.latest_payloads(conn)
    missing = {"municipios", "centroides"} - payloads.keys()
    if missing:
        log.warning("raw do IBGE incompleto (faltam: %s); nada a processar", sorted(missing))
        return None

    frame = transform.to_municipalities(payloads["municipios"], payloads["centroides"])
    if frame.height < MIN_MUNICIPALITIES:
        raise ValueError(
            f"só {frame.height} municípios no raw do IBGE (esperado ≥ {MIN_MUNICIPALITIES})"
        )

    load.replace(conn, frame)

    summary = Summary(
        municipalities=frame.height,
        with_coordinates=frame.drop_nulls(["latitude", "longitude"]).height,
    )
    log.info(
        "municípios: %d, com coordenadas: %d", summary.municipalities, summary.with_coordinates
    )
    return summary
