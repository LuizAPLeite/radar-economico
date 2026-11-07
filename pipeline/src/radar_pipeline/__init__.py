"""Pipeline do Radar Econômico: raw -> staging -> mart."""

import logging

LAYERS = ("raw", "staging", "mart")


def main() -> None:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    logging.getLogger(__name__).info("pipeline iniciado (camadas: %s)", ", ".join(LAYERS))
