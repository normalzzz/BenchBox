#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Measure the time from a BenchBox load request to ECS service scale-out signals.

Required:
  --cluster NAME       ECS cluster name
  --service NAME       ECS service name
  --url URL            BenchBox base URL, for example http://example.com

Optional:
  --cpu PERCENT        cpu_percent for /load, default: 100
  --memory MB          memory_mb for /load, default: 0
  --region REGION      AWS region. Falls back to AWS_REGION/AWS_DEFAULT_REGION
  --timeout SECONDS    Max polling time, default: 600
  --interval SECONDS   Poll interval, default: 2

Example:
  scripts/measure-ecs-scale.sh \
    --cluster benchbox-cluster \
    --service benchbox \
    --url http://benchbox-lb.example.com \
    --cpu 100 \
    --memory 256 \
    --region cn-north-1
EOF
}

CLUSTER=""
SERVICE=""
URL=""
CPU_PERCENT="100"
MEMORY_MB="0"
REGION="${AWS_REGION:-${AWS_DEFAULT_REGION:-}}"
TIMEOUT_SECONDS="600"
POLL_INTERVAL_SECONDS="2"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cluster)
      CLUSTER="$2"
      shift 2
      ;;
    --service)
      SERVICE="$2"
      shift 2
      ;;
    --url)
      URL="${2%/}"
      shift 2
      ;;
    --cpu)
      CPU_PERCENT="$2"
      shift 2
      ;;
    --memory)
      MEMORY_MB="$2"
      shift 2
      ;;
    --region)
      REGION="$2"
      shift 2
      ;;
    --timeout)
      TIMEOUT_SECONDS="$2"
      shift 2
      ;;
    --interval)
      POLL_INTERVAL_SECONDS="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "$CLUSTER" || -z "$SERVICE" || -z "$URL" ]]; then
  usage >&2
  exit 2
fi

AWS_ARGS=()
if [[ -n "$REGION" ]]; then
  AWS_ARGS+=(--region "$REGION")
fi

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 2
  fi
}

now_ms() {
  date +%s%3N
}

elapsed_ms() {
  local now
  now="$(now_ms)"
  echo $((now - START_MS))
}

require_command aws
require_command curl
require_command date

RESOURCE_ID="service/${CLUSTER}/${SERVICE}"
PAYLOAD="{\"cpu_percent\":${CPU_PERCENT},\"memory_mb\":${MEMORY_MB}}"

read -r BASE_DESIRED BASE_RUNNING BASE_PENDING < <(
  aws ecs describe-services \
    "${AWS_ARGS[@]}" \
    --cluster "$CLUSTER" \
    --services "$SERVICE" \
    --query 'services[0].[desiredCount,runningCount,pendingCount]' \
    --output text
)

BASE_ACTIVITY_ID="$(
  aws application-autoscaling describe-scaling-activities \
    "${AWS_ARGS[@]}" \
    --service-namespace ecs \
    --scalable-dimension ecs:service:DesiredCount \
    --resource-id "$RESOURCE_ID" \
    --max-items 1 \
    --query 'ScalingActivities[0].ActivityId' \
    --output text 2>/dev/null || true
)"

if [[ "$BASE_ACTIVITY_ID" == "None" ]]; then
  BASE_ACTIVITY_ID=""
fi

echo "cluster=$CLUSTER service=$SERVICE resource_id=$RESOURCE_ID"
echo "baseline desired=$BASE_DESIRED running=$BASE_RUNNING pending=$BASE_PENDING"
echo "payload=$PAYLOAD"

START_MS="$(now_ms)"
START_ISO="$(date -Iseconds)"

echo "t0 curl_start ${START_ISO}"
curl -fsS -X POST "${URL}/load" \
  -H 'Content-Type: application/json' \
  -d "$PAYLOAD" >/dev/null
echo "t+$(elapsed_ms)ms curl_completed"

ACTIVITY_SEEN_MS=""
DESIRED_CHANGED_MS=""
RUNNING_INCREASED_MS=""
LAST_ACTIVITY_ID=""
DEADLINE_MS=$((START_MS + TIMEOUT_SECONDS * 1000))

while [[ "$(now_ms)" -lt "$DEADLINE_MS" ]]; do
  read -r DESIRED RUNNING PENDING < <(
    aws ecs describe-services \
      "${AWS_ARGS[@]}" \
      --cluster "$CLUSTER" \
      --services "$SERVICE" \
      --query 'services[0].[desiredCount,runningCount,pendingCount]' \
      --output text
  )

  if [[ -z "$DESIRED_CHANGED_MS" && "$DESIRED" =~ ^[0-9]+$ && "$DESIRED" -gt "$BASE_DESIRED" ]]; then
    DESIRED_CHANGED_MS="$(elapsed_ms)"
    echo "t+${DESIRED_CHANGED_MS}ms ecs_desired_count_changed desired=$DESIRED running=$RUNNING pending=$PENDING"
  fi

  if [[ -z "$RUNNING_INCREASED_MS" && "$RUNNING" =~ ^[0-9]+$ && "$RUNNING" -gt "$BASE_RUNNING" ]]; then
    RUNNING_INCREASED_MS="$(elapsed_ms)"
    echo "t+${RUNNING_INCREASED_MS}ms ecs_running_count_increased desired=$DESIRED running=$RUNNING pending=$PENDING"
  fi

  ACTIVITY_ID="$(
    aws application-autoscaling describe-scaling-activities \
      "${AWS_ARGS[@]}" \
      --include-not-scaled-activities \
      --service-namespace ecs \
      --scalable-dimension ecs:service:DesiredCount \
      --resource-id "$RESOURCE_ID" \
      --max-items 1 \
      --query 'ScalingActivities[0].ActivityId' \
      --output text 2>/dev/null || true
  )"

  if [[ "$ACTIVITY_ID" != "None" && -n "$ACTIVITY_ID" && "$ACTIVITY_ID" != "$BASE_ACTIVITY_ID" ]]; then
    if [[ -z "$ACTIVITY_SEEN_MS" ]]; then
      ACTIVITY_SEEN_MS="$(elapsed_ms)"
      echo "t+${ACTIVITY_SEEN_MS}ms scaling_activity_seen activity_id=$ACTIVITY_ID"
    fi

    if [[ "$ACTIVITY_ID" != "$LAST_ACTIVITY_ID" ]]; then
      LAST_ACTIVITY_ID="$ACTIVITY_ID"
      aws application-autoscaling describe-scaling-activities \
        "${AWS_ARGS[@]}" \
        --include-not-scaled-activities \
        --service-namespace ecs \
        --scalable-dimension ecs:service:DesiredCount \
        --resource-id "$RESOURCE_ID" \
        --max-items 1 \
        --query 'ScalingActivities[0].[StartTime,EndTime,StatusCode,Description,Cause,StatusMessage]' \
        --output text
    fi
  fi

  if [[ -n "$ACTIVITY_SEEN_MS" && -n "$DESIRED_CHANGED_MS" && -n "$RUNNING_INCREASED_MS" ]]; then
    break
  fi

  sleep "$POLL_INTERVAL_SECONDS"
done

echo
echo "summary"
echo "curl_start=${START_ISO}"
echo "scaling_activity_seen_ms=${ACTIVITY_SEEN_MS:-not_seen}"
echo "ecs_desired_count_changed_ms=${DESIRED_CHANGED_MS:-not_seen}"
echo "ecs_running_count_increased_ms=${RUNNING_INCREASED_MS:-not_seen}"
