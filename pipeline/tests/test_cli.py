import pytest

from radar_pipeline.cli import parse_flag, select_jobs


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


@pytest.mark.parametrize(
    ("value", "expected"),
    [
        (None, False),
        ("", False),
        ("false", False),
        ("0", False),
        ("true", True),
        (" TRUE ", True),
        ("1", True),
    ],
)
def test_parse_flag(value, expected):
    assert parse_flag("PIPELINE_FULL_REFRESH", value) is expected


def test_parse_flag_rejects_other_values():
    with pytest.raises(ValueError, match="PIPELINE_FULL_REFRESH"):
        parse_flag("PIPELINE_FULL_REFRESH", "sim")
