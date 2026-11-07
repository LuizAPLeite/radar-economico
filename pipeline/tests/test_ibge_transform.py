import pytest
from pandera.errors import SchemaErrors

from radar_pipeline.ibge.transform import to_municipalities


def municipio(code: int, name: str, uf: str) -> dict:
    # Mesmo formato da API de localidades (visão "nivelada").
    return {"municipio-id": code, "municipio-nome": name, "UF-sigla": uf}


def centroide(code: int, lat: float, lon: float) -> dict:
    # Mesmo formato da API de malhas: o id vem como texto.
    return {"id": str(code), "centroide": {"latitude": lat, "longitude": lon}}


def test_joins_municipalities_with_centroids():
    frame = to_municipalities(
        [municipio(3550308, "São Paulo", "SP"), municipio(4317103, "Sant'Ana do Livramento", "RS")],
        [centroide(3550308, -23.6501, -46.6481)],
    )
    assert frame.rows() == [
        (3550308, "São Paulo", "SP", -23.6501, -46.6481),
        # sem centroide publicado: entra com coordenadas nulas
        (4317103, "Sant'Ana do Livramento", "RS", None, None),
    ]


def test_ignores_centroid_entries_without_coordinates():
    frame = to_municipalities(
        [municipio(3550308, "São Paulo", "SP")],
        [{"id": "3550308", "centroide": None}],
    )
    assert frame["latitude"].to_list() == [None]


@pytest.mark.parametrize(
    ("municipios", "centroides"),
    [
        ([municipio(3550308, "São Paulo", "XX")], []),  # UF inexistente
        ([municipio(3550308, "São Paulo", "SP"), municipio(3550308, "São Paulo", "SP")], []),
        ([municipio(355030, "Seis dígitos", "SP")], []),  # código sem 7 dígitos
        (
            [municipio(3550308, "São Paulo", "SP")],
            [centroide(3550308, -46.6, -23.6)],
        ),  # lat/lon trocadas
    ],
)
def test_rejects_invalid_data(municipios, centroides):
    with pytest.raises(SchemaErrors):
        to_municipalities(municipios, centroides)
