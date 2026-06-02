---
name: publish
description: Gera nova versão (SemVer automático), commita tudo e faz push para o remoto — SEM criar tag
---

Publique uma nova versão do projeto atual SEM criar tag. NUNCA pergunte o tipo de bump ao usuário — determine automaticamente analisando o histórico.

## Regras inegociáveis

- **Gerenciador de pacotes**: respeite o lockfile do projeto. `yarn.lock` → `yarn`; `package-lock.json` → `npm`; `pnpm-lock.yaml` → `pnpm`; `bun.lockb` → `bun`.
- **NUNCA** adicionar `Co-Authored-By` ou qualquer assinatura do Claude em commits.
- **NUNCA** usar `--no-verify`, `--no-gpg-sign` ou flags que pulem hooks/assinatura.
- **NUNCA** crie tag neste comando. Se o usuário quiser tag, direcione-o para `/release`.
- **SemVer**: MAJOR.MINOR.PATCH.
- Respeite o idioma predominante dos commits do repositório.

## Passo a passo

### 1. Coletar contexto (em paralelo)

Identifique primeiro o commit que produziu a versão atual do manifest. **Não use `git describe --tags`** — a última tag pode estar várias versões atrás (vários `/publish` sem tag entre `/release`). A referência correta é o último commit que efetivamente bumpou a versão.

A estratégia combina duas heurísticas e escolhe o commit mais recente entre elas:

1. **Grep da mensagem** (`^chore(release):` ou `^release:`): pega o último commit cujo título indica bump.
2. **Pickaxe da linha de versão no manifest**: pega o último commit que de fato modificou a linha de versão. Cobre o caso de **squash merge** (no GitHub, GitLab, etc.), onde o commit consolidado tem título arbitrário (ex.: `"PR #123 — feature X"`) mas o diff incluiu o bump de versão dentro.

Detecte o manifest pelo lockfile/arquivo presente e ajuste o regex pickaxe:
- `package.json` (npm/yarn/pnpm/bun): `'"version"[[:space:]]*:'`
- `Cargo.toml` (Rust): `'^version[[:space:]]*='`
- `pyproject.toml` (Python): `'^version[[:space:]]*='`
- `setup.py`, `setup.cfg`, `composer.json`, `Gemfile`: ajustar o regex conforme a convenção do arquivo.

```bash
# Ajuste MANIFEST e VERSION_PATTERN conforme o tipo de projeto detectado:
MANIFEST=package.json
VERSION_PATTERN='"version"[[:space:]]*:'

GREP_COMMIT=$(git log -1 --pretty=format:%H --grep='^\(chore(release)\|release\):' 2>/dev/null)
PICKAXE_COMMIT=$(git log -1 --pretty=format:%H -G"$VERSION_PATTERN" -- "$MANIFEST" 2>/dev/null)

if [ -n "$GREP_COMMIT" ] && [ -n "$PICKAXE_COMMIT" ]; then
  GREP_TS=$(git show -s --format=%ct "$GREP_COMMIT")
  PICKAXE_TS=$(git show -s --format=%ct "$PICKAXE_COMMIT")
  if [ "$PICKAXE_TS" -gt "$GREP_TS" ]; then
    LAST_VERSION_COMMIT=$PICKAXE_COMMIT
  else
    LAST_VERSION_COMMIT=$GREP_COMMIT
  fi
else
  LAST_VERSION_COMMIT="${GREP_COMMIT:-$PICKAXE_COMMIT}"
fi
[ -z "$LAST_VERSION_COMMIT" ] && LAST_VERSION_COMMIT=$(git rev-list --max-parents=0 HEAD)
```

Em seguida rode em paralelo:
- `git status`
- Leia o manifest do projeto (`package.json`, `Cargo.toml`, `pyproject.toml`, etc.) e extraia a versão atual.
- `git log ${LAST_VERSION_COMMIT}..HEAD --oneline` — commits desde o último bump.
- `git diff ${LAST_VERSION_COMMIT}..HEAD --stat`
- `git diff main...HEAD --stat` (ou `master` conforme o repositório).
- `git branch --show-current`.

### 2. Determinar o bump AUTOMATICAMENTE

Analise os commits desde o último bump (`${LAST_VERSION_COMMIT}..HEAD`). Mesma lógica do `/release` (primeira regra que bater vence):

- **MAJOR**: `BREAKING CHANGE:` no corpo OU `!` após tipo/escopo.
- **MINOR**: pelo menos um `feat(...)` ou `feat:`.
- **PATCH**: apenas `fix`, `chore`, `docs`, `refactor`, `perf`, `style`, `test`, `build`, `ci` ou não-convencionais.

Se não houver commits desde o último bump, avise e pare — não gere commit vazio.

### 3. Atualizar o arquivo de versão

Edite o campo de versão no manifest para a nova versão calculada.

### 4. Stagear arquivos específicos

**NUNCA** use `git add -A` ou `git add .`. Adicione apenas os arquivos realmente modificados. Avise antes de commitar se detectar arquivos sensíveis (`.env*`, `*.key`, `*credentials*`).

### 5. Commitar

Formato: `chore(release): vX.Y.Z` (evite `release: vX.Y.Z` puro para não confundir com o workflow tagged).

```bash
git commit -m "chore(release): vX.Y.Z"
```

Se pre-commit hook falhar, conserte e crie NOVO commit.

### 6. Push (sem tag)

```bash
git push origin HEAD
```

Se não houver upstream, use `git push -u origin <branch>`.

**NÃO** rode `git push --tags` nem `git tag`.

### 7. Reportar

Responda em 2–4 linhas: versão anterior → nova versão, motivo do bump, confirmação de push — destacando que **NENHUMA tag foi criada**.

## Confirmações antes de agir

Mostre ao usuário a versão calculada, o motivo e os arquivos a serem commitados. **Aguarde OK explícito** antes de `git commit` e `git push`.
