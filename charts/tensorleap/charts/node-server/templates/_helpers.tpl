{{/*
Bash body for the regular `.js` db-migration loop. Used by both the
db-migration-pre and db-migration-post init containers; the two passes
differ only in their MIGRATION_FROM / MIGRATION_TO env vars, which
slice this same loop into a "before the special migration" pass and
an "after the special migration" pass.

Reads:
  MONGO_URI          (from configmap)
  POD_NAME           (downward API)
  MIGRATION_FROM     (optional, defaults to 1 = no lower bound)
  MIGRATION_TO       (optional, defaults to 99999 = no upper bound)

Behavior:
  * Loops migrations in /mnt/shared/migrations/*.js sorted by filename.
  * Only runs files whose version is in [FROM, TO] AND strictly greater
    than the current db_metadata.schemaVersion.
  * Uses db_metadata.lock to serialize against concurrent pods.
  * Fresh-install fast path (schemaVersion missing): stamp the global
    LATEST_VERSION and exit, skipping every migration. Skipping is safe
    because there's no legacy data to migrate.
*/}}
{{- define "node-server.dbMigrationScript" -}}
set -euo pipefail

FROM=${MIGRATION_FROM:-1}
TO=${MIGRATION_TO:-99999}

echo 'Verifying db schema version...'
echo "Migration window for this pass: [$FROM..$TO]"

CURRNET_VERSION=$(mongosh $MONGO_URI --eval 'print((db.db_metadata.findOne({ _id: 1 }) || { schemaVersion: 0 }).schemaVersion); process.exit(0);' --quiet)
echo "Current schemaVersion: $CURRNET_VERSION"
cd /mnt/shared/migrations
LATEST_VERSION=$(ls *.js | sed 's/-.*$//' | sort -r | head -1 | sed 's/^0*//')
echo "Latest schemaVersion (all files): $LATEST_VERSION"

TARGET_VERSION=$LATEST_VERSION
if [ $TO -lt $TARGET_VERSION ]; then TARGET_VERSION=$TO; fi
echo "Target schemaVersion for this pass: $TARGET_VERSION"

test $CURRNET_VERSION -ge $TARGET_VERSION && { echo "Already at or above target — nothing to do."; exit 0; }

if [ $CURRNET_VERSION -eq 0 ]
then
  echo "Setting initial schemaVersion to $LATEST_VERSION (fresh install — skipping all migrations)"
  mongosh $MONGO_URI --eval "db.db_metadata.insertOne({ _id: 1, schemaVersion: $LATEST_VERSION })" --quiet
  exit 0
fi

echo "Locking DB"
FAILED_TO_GET_LOCK=$(mongosh $MONGO_URI --eval "db.db_metadata.findOneAndUpdate({_id: 1, schemaVersion: $CURRNET_VERSION, lock: { \$exists: false }}, { \$set: { lock: 'Preparing for migration from $CURRNET_VERSION to $TARGET_VERSION' }})" --quiet)
if [ "$FAILED_TO_GET_LOCK" == 'true' ];
then
  echo "Failed to lock DB! (Could be failed migration), waiting for lock to clear before restarting"
  LOCK_TEXT=$(mongosh $MONGO_URI --eval "db.db_metadata.findOne({ _id: 1 }).lock" --quiet)
  until [ -z "$LOCK_TEXT" ]
  do
    echo "DB Still locked: $LOCK_TEXT"
    echo "To see previous run logs run: kubectl logs $POD_NAME --previous"
    echo "To release the lock run: kubectl exec -it mongodb-0 -- mongo mongodb://127.0.0.1:27017/tensorleap --eval 'db.db_metadata.update({_id:1}, {\$unset: {lock: true}})'"
    sleep 30
    LOCK_TEXT=$(mongosh $MONGO_URI --eval "db.db_metadata.findOne({ _id: 1 }).lock" --quiet)
  done
  echo 'DB Unlocked! restarting...'
  exit -1
fi

for file in $(ls *.js | sort); do
  VERSION=$(echo $file | sed 's/-.*$//' | sed 's/^0*//')
  if [ $VERSION -gt $CURRNET_VERSION ] && [ $VERSION -ge $FROM ] && [ $VERSION -le $TO ]
  then
    echo "Updating lock description..."
    mongosh $MONGO_URI --eval "db.db_metadata.findOneAndUpdate({_id: 1}, { \$set: { lock: 'Migration from $CURRNET_VERSION to $TARGET_VERSION: $file' }}) == null" --quiet
    echo "Running $file"
    mongosh $MONGO_URI -f $file --quiet
    echo "Upating schemaVersion to $VERSION"
    mongosh $MONGO_URI --eval "db.db_metadata.updateOne({ _id: 1 }, { \$set: { schemaVersion: $VERSION }}, { upsert: true })" --quiet
  fi
done

echo "Unlocking DB"
mongosh $MONGO_URI --eval "db.db_metadata.updateOne({ _id: 1 },{ \$unset: { lock: true }})" --quiet
{{- end -}}
