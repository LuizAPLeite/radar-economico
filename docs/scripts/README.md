# Imagens do README

As capturas (`docs/img/*.png`) e o GIF de demonstração (`docs/img/demo.gif`)
são gerados a partir do site publicado, com o Edge sem interface (headless)
controlado pelo Chrome DevTools Protocol. Rodar da raiz do repositório, no
Windows (o caminho do Edge está em `shoot.mjs`):

```powershell
node docs/scripts/shoot.mjs docs/scripts/capturas-claro.json
node docs/scripts/shoot.mjs docs/scripts/capturas-escuro.json
node docs/scripts/shoot.mjs docs/scripts/demo.json
uv run --no-project --with pillow python docs/scripts/make_gif.py docs/scripts/frames docs/img/demo.gif 1000
```

Cada roteiro (`*.json`) é uma lista de passos: abrir uma página, esperar os
gráficos, rolar até um título, clicar num botão, digitar, tirar uma foto ou
guardar um quadro do GIF (com quanto tempo ele fica na tela). O roteiro do
GIF faz uma pergunta de verdade ao assistente (gasta uma da cota diária).
Os quadros (`docs/scripts/frames/`) não vão para o git.
