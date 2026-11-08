"""Números no padrão brasileiro, para as mensagens de aviso."""

# Troca vírgula por ponto e ponto por vírgula de uma vez só.
_SWAP = str.maketrans(",.", ".,")


def br_number(value: float, decimals: int, *, signed: bool = False) -> str:
    """1234.5 → "1.234,50" (com signed=True, "+1.234,50")."""
    sign = "+" if signed else ""
    return f"{value:{sign},.{decimals}f}".translate(_SWAP)
