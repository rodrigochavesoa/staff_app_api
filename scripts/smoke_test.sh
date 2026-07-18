#!/bin/bash
# Smoke test script for validating the staff_app Go API in Staging/Production.
# Usage: ./scripts/smoke_test.sh [API_URL]
# Default API_URL: http://localhost:5000

set -euo pipefail

HOST="${1:-http://localhost:5000}"
SMOKE_FICHA_ID="${SMOKE_FICHA_ID:-999}"
SMOKE_ADMIN_USERNAME="${SMOKE_ADMIN_USERNAME:-${ADMIN_DEFAULT_USERNAME:-admin}}"
SMOKE_ADMIN_PASSWORD="${SMOKE_ADMIN_PASSWORD:-${ADMIN_DEFAULT_PASSWORD:-admin-change-me-immediately}}"
# SMTP reenviar-email expectations:
#   unset/empty → accept success OR controlled SMTP failure (safe for local and staging)
#   true        → require SMTP failure (typical local RC without SMTP)
#   false       → require successful send (staging/prod with real SMTP)
SMOKE_EXPECT_SMTP_FAILURE="${SMOKE_EXPECT_SMTP_FAILURE:-}"

echo "🚀 Starting smoke tests against: $HOST"
echo "Using ficha seed ID: $SMOKE_FICHA_ID"
if [ -n "$SMOKE_EXPECT_SMTP_FAILURE" ]; then
  echo "SMTP expectation: SMOKE_EXPECT_SMTP_FAILURE=$SMOKE_EXPECT_SMTP_FAILURE"
else
  echo "SMTP expectation: accept success or controlled failure"
fi

# 1. Check Health Endpoint
echo "🔍 1. Checking health endpoint..."
HEALTH_RESP=$(curl -s -f "$HOST/health")
echo "Health Response: $HEALTH_RESP"
if ! echo "$HEALTH_RESP" | grep -q '"status":"ok"'; then
    echo "❌ Health check failed!"
    exit 1
fi
if ! echo "$HEALTH_RESP" | grep -q '"database":"connected"'; then
    echo "❌ Database connection check failed!"
    exit 1
fi
echo "✅ Health check passed!"

# 2. Authenticate as admin
echo "🔍 2. Authenticating smoke admin..."
AUTH_PAYLOAD=$(cat <<EOF
{
  "username": "$SMOKE_ADMIN_USERNAME",
  "password": "$SMOKE_ADMIN_PASSWORD"
}
EOF
)

AUTH_RESP=$(curl -s -f -X POST -H "Content-Type: application/json" -d "$AUTH_PAYLOAD" "$HOST/api/v1/auth/login")
TOKEN=$(echo "$AUTH_RESP" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
if [ -z "$TOKEN" ]; then
    echo "❌ Admin authentication failed: token not found."
    exit 1
fi
AUTH_HEADER="Authorization: Bearer $TOKEN"
echo "✅ Admin authenticated successfully!"

# Generate a unique email to avoid constraints
RANDOM_VAL=$((RANDOM % 100000 + 1))
TEST_EMAIL="smoke_${RANDOM_VAL}@example.com"

# 3. Create Aluno
echo "🔍 3. Creating test aluno..."
ALUNO_PAYLOAD=$(cat <<EOF
{
  "nome": "Aluno Smoke Test",
  "idade": 30,
  "sexo": "M",
  "email": "$TEST_EMAIL",
  "telefone": "11999999999",
  "objetivo": "Validação de Staging"
}
EOF
)

ALUNO_RESP=$(curl -s -f -X POST -H "Content-Type: application/json" -H "$AUTH_HEADER" -d "$ALUNO_PAYLOAD" "$HOST/api/v1/alunos")
echo "Aluno Created: $ALUNO_RESP"

# Parse Aluno ID using grep and cut (highly portable, no jq required)
ALUNO_ID=$(echo "$ALUNO_RESP" | grep -o '"id":[0-9]*' | cut -d: -f2)
echo "✅ Aluno created successfully with ID: $ALUNO_ID"

# 4. Create Ficha Web Link
echo "🔍 4. Creating public link for aluno..."
FICHA_PAYLOAD=$(cat <<EOF
{
  "ficha_id": $SMOKE_FICHA_ID,
  "aluno_id": $ALUNO_ID,
  "conteudo": {
    "objetivo": "Resistência",
    "semanas": 4,
    "treinos": []
  }
}
EOF
)

if ! FICHA_RESP=$(curl -s -f -X POST -H "Content-Type: application/json" -H "$AUTH_HEADER" -d "$FICHA_PAYLOAD" "$HOST/api/v1/criar-ficha"); then
    echo "❌ Public link creation failed!"
    echo "Hint: ensure fichas_treino_web contains SMOKE_FICHA_ID=$SMOKE_FICHA_ID, or run with SMOKE_FICHA_ID=<existing_id>."
    exit 1
fi
echo "Ficha Link Response: $FICHA_RESP"

# Parse Ficha Link Hash
HASH=$(echo "$FICHA_RESP" | grep -o '"hash":"[^"]*"' | cut -d'"' -f4)
echo "✅ Public link created successfully with Hash: $HASH"

# 5. Fetch Public Ficha Web JSON
echo "🔍 5. Retrieving public ficha web JSON..."
FICHA_JSON_RESP=$(curl -s -f "$HOST/api/v1/ficha/$HASH/json")
if ! echo "$FICHA_JSON_RESP" | grep -q "$HASH"; then
    echo "❌ Ficha JSON retrieval failed!"
    exit 1
fi
echo "✅ Ficha JSON retrieved successfully!"

# 6. Upload Garmin FIT File
echo "🔍 6. Uploading Garmin FIT activity..."
FIT_FILE="internal/garmin/testdata/fit/1_20260329_195128_22325609284_ACTIVITY.fit"

if [ ! -f "$FIT_FILE" ]; then
    echo "⚠️ FIT test fixture not found at $FIT_FILE. Skipping upload test."
else
    UPLOAD_RESP=$(curl -s -f -X POST \
        -H "$AUTH_HEADER" \
        -F "aluno_id=$ALUNO_ID" \
        -F "file=@$FIT_FILE" \
        "$HOST/api/garmin/upload")
    echo "Upload Response: $UPLOAD_RESP"
    if ! echo "$UPLOAD_RESP" | grep -q '"success":true'; then
        echo "❌ Garmin FIT upload failed!"
        exit 1
    fi
    echo "✅ Garmin FIT upload completed successfully!"
fi

# 7. Retrieve Garmin Chart Dashboard
echo "🔍 7. Retrieving Garmin charts dashboard..."
CHART_RESP=$(curl -s -f -H "$AUTH_HEADER" "$HOST/api/garmin/charts/dashboard/$ALUNO_ID")
if ! echo "$CHART_RESP" | grep -q '"success":true'; then
    echo "❌ Garmin charts dashboard query failed!"
    exit 1
fi
if ! echo "$CHART_RESP" | grep -q "chart_json"; then
    echo "❌ Garmin charts dashboard lacks chart_json!"
    exit 1
fi
echo "✅ Garmin charts dashboard retrieved successfully!"

# 9. Test SVED Calculate
echo "🔍 9. Testing SVED calculate endpoint..."
SVED_PAYLOAD=$(cat <<EOF
{
  "cadencia": "3-0-1-0",
  "repeticoes": 10,
  "rir": 2,
  "series": 3,
  "intervalo": 60
}
EOF
)
SVED_RESP=$(curl -s -f -X POST -H "Content-Type: application/json" -H "$AUTH_HEADER" -d "$SVED_PAYLOAD" "$HOST/api/v1/sved/calcular")
echo "SVED Response: $SVED_RESP"
if ! echo "$SVED_RESP" | grep -q '"ies":'; then
    echo "❌ SVED calculation failed!"
    exit 1
fi
echo "✅ SVED calculation check passed!"

# 10. Test Base de Conhecimento / RAG Search (POST JSON contract)
echo "🔍 10. Testing Base de Conhecimento / RAG search..."
RAG_RESP=$(curl -s -f -X POST -H "$AUTH_HEADER" -H "Content-Type: application/json" \
  -d '{"query":"lombalgia","k":3}' \
  "$HOST/api/v1/admin/consulta-base")
echo "RAG Response: $RAG_RESP"
if echo "$RAG_RESP" | grep -q '"error"'; then
    echo "❌ RAG Search returned an error response!"
    exit 1
fi
if ! echo "$RAG_RESP" | grep -q -i "lombalgia"; then
    echo "❌ RAG Search response does not contain the expected fallback local document!"
    exit 1
fi
echo "✅ RAG Search check passed!"

# 11. Test Admin Relatórios Dashboard
echo "🔍 11. Testing specialized admin reports..."
REPORTS_RESP=$(curl -s -f -H "$AUTH_HEADER" "$HOST/api/v1/admin/relatorios/dashboard")
echo "Reports Response: $REPORTS_RESP"
if ! echo "$REPORTS_RESP" | grep -q "total_exercicios_ativos"; then
    echo "❌ Specialized reports dashboard query failed!"
    exit 1
fi
echo "✅ Specialized reports dashboard check passed!"

# 12. Test Reenviar Email de Anamnese
echo "🔍 12. Testing Reenviar Email de Anamnese..."
EMAIL_RESP=$(curl -s -X POST -H "$AUTH_HEADER" "$HOST/api/v1/admin/alunos/$ALUNO_ID/anamnese/reenviar-email")
echo "Email Response: $EMAIL_RESP"

smtp_failure=false
if echo "$EMAIL_RESP" | grep -Eqi 'desabilitado|SMTP|incompletas'; then
  smtp_failure=true
fi
smtp_success=false
if echo "$EMAIL_RESP" | grep -q '"success":true' && echo "$EMAIL_RESP" | grep -qi 'reenviado com sucesso'; then
  smtp_success=true
fi

case "${SMOKE_EXPECT_SMTP_FAILURE}" in
  true|TRUE|1|yes|YES)
    if [ "$smtp_failure" != true ]; then
      echo "❌ Expected SMTP failure (SMOKE_EXPECT_SMTP_FAILURE=true), got unexpected response!"
      exit 1
    fi
    echo "✅ Reenviar Email check passed (SMTP failure as expected)!"
    ;;
  false|FALSE|0|no|NO)
    if [ "$smtp_success" != true ]; then
      echo "❌ Expected SMTP success (SMOKE_EXPECT_SMTP_FAILURE=false), got unexpected response!"
      exit 1
    fi
    echo "✅ Reenviar Email check passed (SMTP send succeeded)!"
    ;;
  "")
    if [ "$smtp_failure" != true ] && [ "$smtp_success" != true ]; then
      echo "❌ Reenviar Email de Anamnese endpoint returned unexpected result!"
      exit 1
    fi
    if [ "$smtp_success" = true ]; then
      echo "✅ Reenviar Email check passed (SMTP send succeeded)!"
    else
      echo "✅ Reenviar Email check passed (controlled SMTP failure)!"
    fi
    ;;
  *)
    echo "❌ Invalid SMOKE_EXPECT_SMTP_FAILURE='$SMOKE_EXPECT_SMTP_FAILURE' (use true, false, or unset)"
    exit 1
    ;;
esac

# 13. Clean up - Soft Delete Aluno
echo "🔍 13. Cleaning up test Aluno (Soft Delete)..."
DELETE_RESP=$(curl -s -I -X DELETE -H "$AUTH_HEADER" "$HOST/api/v1/alunos/$ALUNO_ID")
HTTP_STATUS=$(echo "$DELETE_RESP" | head -n 1 | cut -d' ' -f2)
if [ "$HTTP_STATUS" != "204" ]; then
    echo "❌ Soft delete failed with HTTP status: $HTTP_STATUS"
    exit 1
fi
echo "✅ Cleanup completed successfully (HTTP 204)!"

echo "🎉 All smoke tests passed successfully against $HOST!"
