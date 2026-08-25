#!/bin/sh
# scrub-history.sh — одноразовый скраб приватных строк из истории git.
#
# КОГДА ЗАПУСКАТЬ: вручную, ОДИН РАЗ, непосредственно ПЕРЕД первым merge/push
# истории develop в публичный репозиторий (main). Повторять только если в
# историю develop снова попали приватные данные.
#
# КАК РАБОТАЕТ: создаёт свежий mirror-клон (рабочее дерево НЕ трогает),
# выполняет git filter-repo --replace-text на целевой ветке, затем проверяет,
# что ни один паттерн из правил не остался в файлах и сообщениях коммитов.
#
# ИСПОЛЬЗОВАНИЕ:
#   sh scripts/scrub-history.sh [BRANCH]          # по умолчанию: develop
# Окружение:
#   RULES  путь к файлу правил (по умолчанию: .scrub-rules.txt)
#   DEST   каталог mirror-клона (по умолчанию: /tmp/dial-up-scrub)
set -u

cd "$(git rev-parse --show-toplevel)"

BRANCH=${1:-develop}
RULES=${RULES:-.scrub-rules.txt}
# абсолютный путь к правилам (нужен после cd в mirror внутри скрипта)
case "$RULES" in /*) ;; *) RULES="$(pwd)/$RULES" ;; esac
DEST=${DEST:-/tmp/dial-up-scrub}

FR=""
if command -v git-filter-repo >/dev/null 2>&1; then
  FR=git-filter-repo
elif [ -x "$HOME/Library/Python/3.11/bin/git-filter-repo" ]; then
  FR="$HOME/Library/Python/3.11/bin/git-filter-repo"
fi
if [ -z "$FR" ]; then
  echo "ERROR: git-filter-repo not found. Install: brew install git-filter-repo (или pip3 install --user git-filter-repo)" >&2
  exit 1
fi
[ -f "$RULES" ] || { echo "ERROR: файл правил не найден: $RULES" >&2; exit 1; }

echo "==> mirror-клон в $DEST ..."
rm -rf "$DEST"
git clone --mirror . "$DEST" || exit 1

echo "==> скраб ветки $BRANCH по правилам из $RULES ..."
( cd "$DEST" && "$FR" --replace-text "$RULES" --replace-message "$RULES" --refs "refs/heads/$BRANCH" --force ) || exit 1

echo "==> проверка остатков (должно быть пусто) ..."
LEFT=0
while IFS= read -r line; do
  case "$line" in ''|'#'*) continue ;; esac
  pat=${line%%==>*}
  if git -C "$DEST" log "$BRANCH" --format="%H" --fixed-strings --grep="$pat" | grep -q .; then
    echo "  ОСТАТОК в сообщении коммита: $pat" >&2
    LEFT=$((LEFT+1))
  fi
  if git -C "$DEST" grep -l -F -e "$pat" "$BRANCH" 2>/dev/null | grep -q .; then
    echo "  ОСТАТОК в файлах: $pat" >&2
    LEFT=$((LEFT+1))
  fi
done < "$RULES"

if [ "$LEFT" -gt 0 ]; then
  echo "FAIL: найдено остатков: $LEFT — проверьте $DEST перед push" >&2
  exit 1
fi

ORIGIN=$(git remote get-url origin 2>/dev/null || true)
if [ -n "$ORIGIN" ]; then
  git -C "$DEST" remote set-url origin "$ORIGIN"
  echo "==> origin восстановлен в mirror: $ORIGIN"
fi

cat <<EOF
==> ГОТОВО. История переписана в: $DEST
    Дальше (НЕ пушить из рабочего репозитория!):
      1. просмотр:  git -C $DEST log --oneline -10
      2. push:      git -C $DEST push --force origin $BRANCH
      3. синк:      (в рабочем репо) git fetch --all && git reset --hard origin/$BRANCH
         worktree'ы: git worktree list / remove при необходимости
    ВНИМАНИЕ: хеши коммитов изменились — ветки/PR поверх них нужно перебазировать.
EOF
