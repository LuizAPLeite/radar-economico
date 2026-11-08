from radar_pipeline import runs


def test_describe_error_keeps_short_messages():
    assert runs.describe_error(KeyError("uf")) == "KeyError: 'uf'"


def test_describe_error_cuts_long_messages():
    # Um erro do pandera com milhares de casos não pode inchar o registro.
    text = runs.describe_error(ValueError("x" * 5_000))
    assert len(text) == runs.MAX_ERROR_CHARS
    assert text.startswith("ValueError: xxx")
    assert text.endswith("…")
