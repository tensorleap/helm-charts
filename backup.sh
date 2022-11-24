set -euo pipefail

BACKUP_NAME=backup_${RANDOM}
echo Backup Name: $BACKUP_NAME

echo Backing up MongoDB
kubectl exec -it svc/mongodb -- mongodump -d tensorleap -o /tl/backup/$BACKUP_NAME.tgz --gzip

echo Backing up Minio
kubectl exec -it svc/tensorleap-minio -- cp -vr /export/session /export/$BACKUP_NAME

echo Creating elasticsearch snapshot repository
kubectl exec -it svc/elasticsearch-master -- curl --fail \
  127.0.0.1:9200/_snapshot/backups \
  -X PUT -H 'Content-Type:application/json' \
  -d "{\"type\":\"fs\",\"settings\":{\"location\":\"/usr/share/elasticsearch/data/backup_snapshots\",\"compress\":true}}"

echo Backing up elasticsearch...
kubectl exec -it svc/elasticsearch-master -- curl --fail \
  127.0.0.1:9200/_snapshot/backups/$BACKUP_NAME?wait_for_completion=true \
  -X PUT -H 'Content-Type:application/json' | jq .

