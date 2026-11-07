-- Camadas do pipeline de dados:
--   raw     -> dados brutos exatamente como vieram das fontes (escrito pelo collector)
--   staging -> dados limpos, tipados e validados (escrito pelo pipeline)
--   mart    -> tabelas prontas para consumo (escrito pelo pipeline, lido pela API)
CREATE SCHEMA IF NOT EXISTS raw;
CREATE SCHEMA IF NOT EXISTS staging;
CREATE SCHEMA IF NOT EXISTS mart;

COMMENT ON SCHEMA raw IS 'Dados brutos das fontes, sem transformação';
COMMENT ON SCHEMA staging IS 'Dados limpos e validados';
COMMENT ON SCHEMA mart IS 'Dados prontos para consumo pela API';
