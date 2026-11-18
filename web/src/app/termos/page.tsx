import type { Metadata } from "next";
import Link from "next/link";

import { LegalPage } from "@/components/LegalPage";

export const metadata: Metadata = {
  title: "Termos de uso",
  description: "Condições de uso do Radar Econômico, fontes dos dados e limites das análises e do assistente.",
};

const REPO = "https://github.com/LuizAPLeite/radar-economico";

export default function TermsPage() {
  return (
    <LegalPage title="Termos de uso" updated="18 de novembro de 2026">
      <p>
        Ao usar o Radar Econômico, você concorda com estes termos. Se não concordar, não use o site. Para o
        tratamento de dados pessoais, vale a <Link href="/privacidade">política de privacidade</Link>.
      </p>

      <h2>O que é o serviço</h2>
      <p>
        Um projeto pessoal, gratuito e de código aberto, feito como portfólio, que reúne dados públicos da economia
        brasileira (juros, inflação, câmbio, petróleo e preços de combustíveis) em gráficos, mapas, análises
        estatísticas e um assistente que responde perguntas sobre esses dados. Não há cadastro, pagamento nem
        contratação de nada: por isso não há política de reembolso.
      </p>

      <h2>Não é recomendação financeira</h2>
      <p>
        Nada no site é recomendação de investimento, de compra ou de qualquer decisão financeira. As análises e a
        previsão de preços são exercícios estatísticos, mostrados junto com o erro que tiveram no passado, e podem
        errar. Decisões tomadas com base no site são de responsabilidade de quem as toma.
      </p>

      <h2>Os dados podem ter atraso ou erro</h2>
      <p>
        Os números vêm de fontes oficiais, coletados automaticamente todos os dias e validados antes de aparecer,
        mas podem ficar atrasados quando uma fonte não publica, e podem conter erros das próprias fontes ou do
        processamento. Para uso oficial, consulte sempre a fonte original, indicada em cada página. A situação de
        cada conjunto de dados fica pública na{" "}
        <a href="https://radar-economico-api.onrender.com/health/data">página de saúde dos dados</a>.
      </p>

      <h2>O assistente pode errar</h2>
      <p>
        As respostas do assistente são montadas por um modelo de linguagem (Gemini, do Google), que pode interpretar
        mal a pergunta. Cada resposta mostra a consulta usada, para você conferir. Há limites de perguntas por pessoa
        e por dia, e o assistente pode ficar indisponível.
      </p>

      <h2>Uso aceitável</h2>
      <ul>
        <li>Não tente sobrecarregar o site ou a API, nem contornar os limites de uso.</li>
        <li>
          Testes de segurança só dentro da <a href={`${REPO}/security/policy`}>política de segurança</a>; falhas
          devem ser relatadas pelo canal privado, não exploradas.
        </li>
        <li>Não escreva dados pessoais, seus ou de outras pessoas, nas perguntas ao assistente.</li>
      </ul>

      <h2>Fontes e atribuição</h2>
      <ul>
        <li>
          <a href="https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos">ANP</a>: Levantamento de Preços
          de Combustíveis (dados abertos).
        </li>
        <li>
          <a href="https://www3.bcb.gov.br/sgspub/">Banco Central do Brasil</a>: Sistema Gerenciador de Séries
          Temporais (SGS).
        </li>
        <li>
          <a href="https://www.ibge.gov.br/">IBGE</a>: municípios e malha dos estados.
        </li>
        <li>
          Petróleo Brent: série da EIA (governo dos Estados Unidos), obtida pelo{" "}
          <a href="https://fred.stlouisfed.org/series/DCOILBRENTEU">FRED</a>, do Federal Reserve Bank of St. Louis.
        </li>
      </ul>
      <p>
        Os dados continuam sendo das fontes; o site os reutiliza citando a origem. A fonte tipográfica Archivo é
        distribuída sob a SIL Open Font License. O site não usa fotos nem ilustrações de terceiros.
      </p>

      <h2>Sem garantias</h2>
      <p>
        O site é oferecido como está, sem garantia de disponibilidade, exatidão ou adequação a um propósito
        específico, e pode mudar ou sair do ar a qualquer momento. Na medida permitida pela lei, o mantenedor não
        responde por danos decorrentes do uso ou da impossibilidade de uso do site.
      </p>

      <h2>Lei aplicável e mudanças</h2>
      <p>
        Estes termos seguem a lei brasileira. Podem mudar quando o site mudar; a data no topo indica a versão em
        vigor, e o histórico fica no <a href={REPO}>repositório</a>.
      </p>
    </LegalPage>
  );
}
