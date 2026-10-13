#!/usr/bin/env bash
# Reseed one subject in the local database.
set -euo pipefail

SUBJECT_EMAIL="${1:?usage: reseed-subject.sh <email>}"

psql -h postgres.thirdfort.test -U client -d client \
  -c "DELETE FROM subjects WHERE email = '$SUBJECT_EMAIL'"

if [[ -n "${DRY_RUN:-}" ]]; then
  echo "would reseed $SUBJECT_EMAIL"
  exit 0
fi

./local/seed-subject.sh --email="$SUBJECT_EMAIL" --name "Jane Okafor" jane.okafor@example.com
cat <<LOG >> /tmp/reseed.log
reseeded $SUBJECT_EMAIL for Jane Okafor
LOG
