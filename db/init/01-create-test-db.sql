-- Banco SEPARADO para os testes de integração. Os testes apontam TEST_DATABASE_URL para
-- `velarum_test` (não para `velarum`, o banco do app/dev), de modo que NUNCA poluem o mundo de
-- desenvolvimento. As migrations são aplicadas neste banco pelos próprios testes (pg.Migrate).
-- Roda só na PRIMEIRA inicialização do volume (docker-entrypoint-initdb.d).
CREATE DATABASE velarum_test;
