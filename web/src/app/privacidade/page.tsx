import type { Metadata } from "next";

import { LegalPage } from "@/components/LegalPage";

export const metadata: Metadata = {
  title: "Política de privacidade",
  description: "Quais dados o Radar Econômico trata, por quê, com quem e quais são os seus direitos (LGPD).",
};

const REPO = "https://github.com/LuizAPLeite/radar-economico";

export default function PrivacyPage() {
  return (
    <LegalPage title="Política de privacidade" updated="18 de novembro de 2026">
      <p>
        O Radar Econômico é um projeto pessoal, gratuito e de código aberto, que mostra dados públicos da economia
        brasileira. Não tem cadastro, login, venda, publicidade nem newsletter. Esta página explica, nos termos da Lei
        Geral de Proteção de Dados (Lei nº 13.709/2018, a LGPD), o pouco que o site trata sobre quem o visita.
      </p>

      <h2>Quem é o responsável</h2>
      <p>
        O controlador dos dados é o mantenedor do projeto, pessoa física, identificado pelo perfil{" "}
        <a href="https://github.com/LuizAPLeite">github.com/LuizAPLeite</a>. Pedidos sobre seus dados podem ser
        feitos pelo <a href={`${REPO}/issues`}>repositório do projeto</a> ou, para assuntos que não devam ser
        públicos, pelo canal privado descrito na <a href={`${REPO}/security/policy`}>política de segurança</a>.
      </p>

      <h2>O que é tratado, e por quê</h2>
      <ul>
        <li>
          <strong>Dados técnicos de acesso</strong> (endereço IP, navegador, página e horário): registrados
          automaticamente pelos provedores que hospedam o site e a API, para entregar as páginas, manter o serviço
          no ar e investigar abusos. O código do Radar Econômico não grava o seu IP em lugar nenhum; ele só o usa,
          por no máximo um minuto e apenas na memória, para limitar quantas perguntas o assistente recebe de um
          mesmo endereço. Base legal: legítimo interesse (art. 7º, IX), na segurança e no funcionamento do site.
        </li>
        <li>
          <strong>Perguntas ao assistente</strong>: o texto que você digita na página do assistente é enviado ao
          Google (Gemini) para virar uma consulta aos dados, e a resposta fica guardada na memória do servidor por
          até 24 horas, para que a mesma pergunta não gaste a cota de novo. Os registros (logs) do servidor guardam
          só o tamanho e uma impressão digital (hash) da pergunta, nunca o texto. Base legal: legítimo interesse,
          para responder ao que você pediu.
        </li>
      </ul>
      <p>
        <strong>Não escreva dados pessoais nas perguntas.</strong> O assistente usa o plano gratuito da API do
        Gemini, em que o Google pode usar o conteúdo enviado para melhorar os produtos dele, inclusive com revisão
        humana, conforme os{" "}
        <a href="https://ai.google.dev/gemini-api/terms">termos da API do Gemini</a>.
      </p>

      <h2>Cookies e rastreamento</h2>
      <p>
        O site não usa cookies, nem próprios nem de terceiros, e não tem ferramentas de análise de audiência (como
        Google Analytics), pixels de anúncio ou scripts de rastreamento. Por isso não há banner de consentimento:
        não existe nada a consentir. Se isso mudar, esta política e o site passam a pedir o seu consentimento antes.
      </p>

      <h2>Com quem os dados são compartilhados</h2>
      <p>Somente com os provedores que fazem o site funcionar, cada um com a própria política:</p>
      <ul>
        <li>
          <a href="https://vercel.com/legal/privacy-policy">Vercel</a>: hospeda o site.
        </li>
        <li>
          <a href="https://render.com/privacy">Render</a> e <a href="https://www.cloudflare.com/privacypolicy/">Cloudflare</a>:
          hospedam e protegem a API.
        </li>
        <li>
          <a href="https://policies.google.com/privacy">Google</a>: recebe o texto das perguntas ao assistente.
        </li>
      </ul>
      <p>
        O banco de dados do projeto guarda só dados públicos da economia, nenhum dado de visitantes. Nada é vendido
        ou cedido para publicidade.
      </p>

      <h2>Transferência internacional</h2>
      <p>
        Esses provedores podem processar dados fora do Brasil, principalmente nos Estados Unidos. A transferência
        se apoia nas garantias contratuais que eles oferecem aos clientes (art. 33 da LGPD).
      </p>

      <h2>Por quanto tempo</h2>
      <p>
        Perguntas e respostas ficam na memória do servidor por até 24 horas, e o IP usado no limite de perguntas,
        por até um minuto. Tudo isso some quando o servidor reinicia. Os registros técnicos dos provedores seguem a
        retenção de cada um, em geral de poucos dias.
      </p>

      <h2>Seus direitos</h2>
      <p>
        A LGPD garante a você, entre outros, os direitos de confirmar se tratamos seus dados, acessá-los, corrigi-los,
        pedir que sejam eliminados e saber com quem foram compartilhados (art. 18). Como o projeto não identifica
        visitantes, na maioria dos casos não haverá dado seu para mostrar ou apagar, mas o pedido será respondido.
        Você também pode reclamar à{" "}
        <a href="https://www.gov.br/anpd">Autoridade Nacional de Proteção de Dados (ANPD)</a>.
      </p>

      <h2>Segurança</h2>
      <p>
        O site e a API usam HTTPS, cabeçalhos de segurança e acesso mínimo ao banco. As medidas e as varreduras de
        segurança estão documentadas no <a href={`${REPO}/blob/main/docs/security/relatorio.md`}>relatório de
        segurança</a> do projeto.
      </p>

      <h2>Crianças e adolescentes</h2>
      <p>O site não é direcionado a menores de idade e não coleta dados que permitam identificar quem o visita.</p>

      <h2>Mudanças</h2>
      <p>
        Esta política pode mudar quando o site mudar. A data no topo indica a última versão, e o histórico completo
        fica no <a href={`${REPO}/commits/main/web/src/app/privacidade/page.tsx`}>repositório</a>.
      </p>
    </LegalPage>
  );
}
