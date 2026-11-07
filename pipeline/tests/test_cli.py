import pytest

from radar_pipeline.cli import select_jobs


@pytest.mark.parametrize(
    ("value", "expected"),
    [
        (None, ["bcb", "ibge", "anp"]),
        ("", ["bcb", "ibge", "anp"]),
        ("anp", ["anp"]),
        # A ordem é sempre a de dependência (o IBGE antes da ANP), não a pedida.
        ("anp, ibge", ["ibge", "anp"]),
        ("bcb,bcb", ["bcb"]),
    ],
)
def test_select_jobs(value, expected):
    assert select_jobs(value) == expected


def test_select_jobs_rejects_unknown_names():
    with pytest.raises(ValueError, match="combustiveis"):
        select_jobs("bcb,combustiveis")
