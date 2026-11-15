# Etapa 1: instala as dependências num ambiente virtual com o uv.
FROM python:3.13-slim AS build
COPY --from=ghcr.io/astral-sh/uv:0.13.7 /uv /bin/uv
WORKDIR /app
ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_PYTHON_DOWNLOADS=never

# Dependências primeiro, código depois: a camada das dependências é
# reaproveitada enquanto o uv.lock não mudar.
COPY pyproject.toml uv.lock .python-version ./
RUN uv sync --locked --no-dev --no-install-project

COPY src ./src
# --no-editable instala o próprio pacote dentro do .venv, então a etapa
# final só precisa copiar o .venv.
RUN uv sync --locked --no-dev --no-editable

# Etapa 2: imagem final, sem o uv e sem o código-fonte solto.
FROM python:3.13-slim
# Correções de segurança do Debian (ex.: zlib) e sem o pip do sistema: o
# programa roda do .venv, e o pip traz bibliotecas próprias (urllib3,
# msgpack) que o Trivy acusa e que nunca seriam usadas aqui.
RUN apt-get update && apt-get upgrade -y && rm -rf /var/lib/apt/lists/*     && python -m pip uninstall -y pip setuptools wheel     && useradd --create-home --uid 10001 app
COPY --from=build /app/.venv /app/.venv
ENV PATH="/app/.venv/bin:$PATH" \
    PYTHONUNBUFFERED=1
USER app
ENTRYPOINT ["radar-pipeline"]
