# Etapa 1: dependências. Copiar só os manifestos primeiro faz o Docker
# reaproveitar esta camada enquanto o package-lock.json não mudar.
FROM node:26-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci

# Etapa 2: build. Não precisa da API: a parte que depende dela é gerada a
# cada requisição (ver src/app/page.tsx).
FROM node:26-alpine AS build
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

# Etapa 3: só o servidor standalone, os arquivos estáticos e a pasta public,
# sem node_modules completo nem código-fonte.
FROM node:26-alpine
WORKDIR /app
ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    PORT=3000 \
    HOSTNAME=0.0.0.0
COPY --from=build --chown=node:node /app/.next/standalone ./
COPY --from=build --chown=node:node /app/.next/static ./.next/static
COPY --from=build --chown=node:node /app/public ./public
USER node
EXPOSE 3000
CMD ["node", "server.js"]
