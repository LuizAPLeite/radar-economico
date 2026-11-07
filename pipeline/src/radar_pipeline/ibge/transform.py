"""Transformação pura dos JSONs do IBGE em uma tabela de municípios."""

from typing import Any

import polars as pl

from radar_pipeline.ibge.schemas import StagingMunicipalities

_MUNICIPALITIES = {"ibge_code": pl.Int64, "name": pl.String, "uf": pl.String}
_CENTROIDS = {"ibge_code": pl.Int64, "latitude": pl.Float64, "longitude": pl.Float64}


def to_municipalities(
    municipios: list[dict[str, Any]], centroides: list[dict[str, Any]]
) -> pl.DataFrame:
    """Junta a lista de municípios (visão "nivelada" da API de localidades)
    com os centroides (API de malhas). Município sem centroide fica com
    latitude e longitude nulas: aparece nas tabelas, mas não no mapa."""
    municipalities = pl.DataFrame(
        [(int(m["municipio-id"]), m["municipio-nome"], m["UF-sigla"]) for m in municipios],
        schema=_MUNICIPALITIES,
        orient="row",
    )
    centroids = pl.DataFrame(
        [
            (int(c["id"]), c["centroide"]["latitude"], c["centroide"]["longitude"])
            for c in centroides
            if c.get("centroide")
        ],
        schema=_CENTROIDS,
        orient="row",
    )
    frame = municipalities.join(centroids, on="ibge_code", how="left").sort("ibge_code")
    return StagingMunicipalities.validate(frame, lazy=True)
