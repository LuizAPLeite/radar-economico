# Política de segurança

## Como reportar uma vulnerabilidade

Se você encontrou uma falha de segurança no Radar Econômico (no código deste
repositório, no site ou na API publicados), **não abra uma issue pública**.
Use o reporte privado do GitHub:

1. Vá em **Security → Report a vulnerability** neste repositório.
2. Descreva o problema, como reproduzir e o impacto que você imagina.

Resposta inicial em até 7 dias. Se a falha se confirmar, ela é corrigida antes
de qualquer detalhe ser publicado, e você recebe o crédito no relatório (se
quiser).

## Escopo

- Código deste repositório.
- Site: https://radar-economico-phi.vercel.app
- API: https://radar-economico-api.onrender.com

Fora do escopo: testes de carga ou negação de serviço, engenharia social e
os serviços de terceiros que o projeto usa (Vercel, Render, Neon, Google,
fontes de dados públicas).

## Versões suportadas

Só a versão em produção, que é a da branch `main`.

## Como o projeto cuida da segurança

O modelo de ameaças está em [docs/security/threat-model.md](docs/security/threat-model.md).
O relatório das varreduras e correções, com o antes e depois, está em
[docs/security/relatorio.md](docs/security/relatorio.md).
