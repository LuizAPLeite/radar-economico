"use server";

import { askAssistant, type Answer } from "@/lib/api";
import { errorMessage, validateQuestion } from "@/lib/assistant";

/** Estado do formulário: a última pergunta e a resposta ou o erro dela. */
export interface AskState {
  question: string;
  answer: Answer | null;
  error: string | null;
}

/**
 * Server Action do formulário: roda no servidor do Next, que repassa a
 * pergunta à API (o navegador nunca fala com a API). Como toda Server Action,
 * é uma entrada pública: confere a pergunta antes de gastar a cota do LLM e
 * devolve só o que a página mostra.
 */
export async function ask(_previous: AskState, formData: FormData): Promise<AskState> {
  const raw = formData.get("question");
  const question = typeof raw === "string" ? raw.trim() : "";

  const invalid = validateQuestion(question);
  if (invalid) {
    return { question, answer: null, error: invalid };
  }
  try {
    const result = await askAssistant(question);
    return result.ok
      ? { question, answer: result.answer, error: null }
      : { question, answer: null, error: errorMessage(result.code) };
  } catch {
    // Rede ou API fora do ar: o detalhe não interessa ao visitante.
    return { question, answer: null, error: errorMessage("unavailable") };
  }
}
