#!/bin/bash
#set -euo pipefail

BASE_URL="http://127.0.0.1:7080"
DELETE_AFTER_INSERT=false
COUNT_PER_USER=30       # records in example1 and example2 per user
REL_PER_USER=12         # relational rows per user

command -v jq >/dev/null || { echo "jq is required"; exit 1; }

# Authenticate and return .token as plain string
login() {
  local username="$1"
  local password="$2"
  curl -s -X POST "$BASE_URL/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$username\",\"password\":\"$password\"}" \
  | jq -r '.token'
}

# Create user as admin
create_user() {
  local admin_header="$1"
  local username="$2"
  local password="$3"
  local email="$4"
  curl -s -X POST "$BASE_URL/user" \
    -H "$admin_header" -H "Content-Type: application/json" \
    -d "{
      \"username\":\"$username\",
      \"password\":\"$password\",
      \"role\":\"user\",
      \"email\":\"$email\",
      \"email_verified\":true
    }" >/dev/null
}

# Create API key for the current user
create_api_key() {
  local auth_header="$1"
  curl -s -X POST "$BASE_URL/me/api-key" \
    -H "$auth_header" \
  | jq -r '.api_key'
}

# Insert data for a user into example1 and example2
insert_data_for_user() {
  local auth_header="$1"
  local prefix="$2"
  local n="${3:-20}"

  for i in $(seq -w 1 "$n"); do
    local id1="${prefix}_ex1_${i}"
    local id2="${prefix}_ex2_${i}"

    # example1 field2 alternates between email and base64 image
    if (( 10#$i % 5 == 0 )); then
      local f2_ex1="$(get_random_image)"
    else
      local f2_ex1="${prefix}.${i}@example.com"
    fi

    # example2 field2 alternates between department name and base64 image
    if (( 10#$i % 7 == 0 )); then
      local f2_ex2="$(get_random_image)"
    else
      local f2_ex2="Department ${i} for ${prefix}"
    fi

    curl -s -X POST "$BASE_URL/example1" \
      -H "$auth_header" -H "Content-Type: application/json" \
      -d "{\"field1\":\"$id1\",\"field2\":\"$f2_ex1\"}" >/dev/null

    curl -s -X POST "$BASE_URL/example2" \
      -H "$auth_header" -H "Content-Type: application/json" \
      -d "{\"field1\":\"$id2\",\"field2\":\"$f2_ex2\"}" >/dev/null
  done
}

# Create relational entries owned by the user
insert_rel_for_user() {
  local auth_header="$1"
  local prefix="$2"
  local n="${3:-10}"

  for i in $(seq -w 1 "$n"); do
    local id1="${prefix}_ex1_${i}"
    local id2="${prefix}_ex2_${i}"
    local note="Relation ${i} for ${prefix}"
    curl -s -X POST "$BASE_URL/exampleRelational" \
      -H "$auth_header" -H "Content-Type: application/json" \
      -d "{\"example1_field1\":\"$id1\",\"example2_field1\":\"$id2\",\"field3\":\"$note\"}" >/dev/null
  done
}

# Get total items for a resource using pagination meta
get_total_items() {
  local auth_header="$1"
  local resource="$2"
  curl -s -X GET "$BASE_URL/${resource}?page_size=1000" \
    -H "$auth_header" \
  | jq -r '.meta.total_items'
}

# Print a short sample for a resource
print_sample() {
  local auth_header="$1"
  local resource="$2"
  local label="$3"
  echo "Sample for $label on $resource"
  curl -s -X GET "$BASE_URL/${resource}?page_size=5" \
    -H "$auth_header" \
  | jq -r '.data[]'
  echo ""
}

# Try fetching a specific record by id
get_by_id() {
  local auth_header="$1"
  local resource="$2"
  local id="$3"
  curl -s -o /dev/stderr -w "%{http_code}" "$BASE_URL/${resource}/${id}" \
    -H "$auth_header"
}

# Try a PATCH to change field2 and check if it is allowed
patch_record() {
  local auth_header="$1"
  local resource="$2"
  local id="$3"
  local new_value="$4"
  curl -s -o /dev/stderr -w "%{http_code}" -X PATCH "$BASE_URL/${resource}/${id}" \
    -H "$auth_header" -H "Content-Type: application/json" \
    -d "{\"field2\":\"$new_value\"}"
}

# Stats endpoint
get_stats_count() {
  local auth_header="$1"
  curl -s "$BASE_URL/stats" -H "$auth_header" | jq -r '.data | length'
}

# Keep original images
BASE64_IMAGE1="data:image/png;base64,/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/2wBDAQkJCQwLDBgNDRgyIRwhMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjL/wAARCABkAMgDASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDxyiiiv3E8w..."
BASE64_IMAGE2="data:image/png;base64,/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/2wBDAQkJCQwLDBgNDRgyIRwhMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjL/wAARCABkAMgDASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDiqKKK+aPjwoooo..."
BASE64_IMAGE3="data:image/png;base64,/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/2wBDAQkJCQwLDBgNDRgyIRwhMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjL/wAARCABkAMgDASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDzSiiivpT7A..."
IMAGES=("$BASE64_IMAGE1" "$BASE64_IMAGE2" "$BASE64_IMAGE3")

get_random_image() { echo "${IMAGES[$((RANDOM % 3))]}"; }

# Login as admin
ADMIN_JWT="$(login "admin" "admin")"
ADMIN_AUTH="Authorization: Bearer $ADMIN_JWT"

# Create users
USER_A="alice"
USER_B="bob"
PASS_A="passalice"
PASS_B="passbob"
echo "Creating users $USER_A and $USER_B"
create_user "$ADMIN_AUTH" "$USER_A" "$PASS_A" "alice@example.com" || true
create_user "$ADMIN_AUTH" "$USER_B" "$PASS_B" "bob@example.com" || true

 

# Login as users
JWT_A="$(login "$USER_A" "$PASS_A")"
JWT_B="$(login "$USER_B" "$PASS_B")"
AUTH_A="Authorization: Bearer $JWT_A"
AUTH_B="Authorization: Bearer $JWT_B"
 
# Insert per user
echo "Inserting data for $USER_A and $USER_B"
insert_data_for_user "$AUTH_A" "$USER_A" "$COUNT_PER_USER"
insert_data_for_user "$AUTH_B" "$USER_B" "$COUNT_PER_USER"

 

# Create relational rows
insert_rel_for_user "$AUTH_A" "$USER_A" "$REL_PER_USER"
insert_rel_for_user "$AUTH_B" "$USER_B" "$REL_PER_USER"
 
# Visibility checks
E1_A="$(get_total_items "$AUTH_A" "example1")"
E2_A="$(get_total_items "$AUTH_A" "example2")"
ER_A="$(get_total_items "$AUTH_A" "exampleRelational")"
 
E1_B="$(get_total_items "$AUTH_B" "example1")"
E2_B="$(get_total_items "$AUTH_B" "example2")"
ER_B="$(get_total_items "$AUTH_B" "exampleRelational")"
 
E1_ADMIN="$(get_total_items "$ADMIN_AUTH" "example1")"
E2_ADMIN="$(get_total_items "$ADMIN_AUTH" "example2")"
ER_ADMIN="$(get_total_items "$ADMIN_AUTH" "exampleRelational")"
 
echo ""
echo "Visibility summary"
echo "$USER_A sees example1: $E1_A items"
echo "$USER_A sees example2: $E2_A items"
echo "$USER_A sees exampleRelational: $ER_A items"
echo "$USER_B sees example1: $E1_B items"
echo "$USER_B sees example2: $E2_B items"
echo "$USER_B sees exampleRelational: $ER_B items"
echo "admin sees example1: $E1_ADMIN items"
echo "admin sees example2: $E2_ADMIN items"
echo "admin sees exampleRelational: $ER_ADMIN items"
echo ""
echo "Expected per user: example1=$COUNT_PER_USER, example2=$COUNT_PER_USER, exampleRelational=$REL_PER_USER"
echo ""
 
# Print small samples including created_by
print_sample "$AUTH_A" "example1" "$USER_A"
print_sample "$AUTH_B" "example1" "$USER_B"
print_sample "$AUTH_A" "example2" "$USER_A"
print_sample "$AUTH_B" "example2" "$USER_B"
print_sample "$AUTH_A" "exampleRelational" "$USER_A"
print_sample "$AUTH_B" "exampleRelational" "$USER_B"
 
# By-id visibility tests
ID_ALICE_ONE="${USER_A}_ex1_01"
ID_BOB_ONE="${USER_B}_ex1_01"
echo "Get-by-id tests"
echo -n "$USER_A tries own $ID_ALICE_ONE on example1 -> "
get_by_id "$AUTH_A" "example1" "$ID_ALICE_ONE"; echo
echo -n "$USER_A tries $USER_B record $ID_BOB_ONE on example1 -> "
get_by_id "$AUTH_A" "example1" "$ID_BOB_ONE"; echo
echo -n "$USER_B tries own $ID_BOB_ONE on example1 -> "
get_by_id "$AUTH_B" "example1" "$ID_BOB_ONE"; echo
echo -n "$USER_B tries $USER_A record $ID_ALICE_ONE on example1 -> "
get_by_id "$AUTH_B" "example1" "$ID_ALICE_ONE"; echo
echo ""

# Patch tests
echo "Patch tests"
echo -n "$USER_A patches own $ID_ALICE_ONE -> "
patch_record "$AUTH_A" "example1" "$ID_ALICE_ONE" "updated_by_${USER_A}"; echo
echo -n "$USER_A patches $USER_B record $ID_BOB_ONE -> "
patch_record "$AUTH_A" "example1" "$ID_BOB_ONE" "should_fail_${USER_A}"; echo
echo -n "$USER_B patches own $ID_BOB_ONE -> "
patch_record "$AUTH_B" "example1" "$ID_BOB_ONE" "updated_by_${USER_B}"; echo
echo -n "$USER_B patches $USER_A record $ID_ALICE_ONE -> "
patch_record "$AUTH_B" "example1" "$ID_ALICE_ONE" "should_fail_${USER_B}"; echo
echo ""
 
# Stats scope checks
echo "Stats visibility"
echo "$USER_A sees $(get_stats_count "$AUTH_A") tables in /stats"
echo "$USER_B sees $(get_stats_count "$AUTH_B") tables in /stats"
echo "admin sees $(get_stats_count "$ADMIN_AUTH") tables in /stats"
echo ""
 
# API key auth test for alice
echo "Creating API key for $USER_A"
API_KEY_A="$(create_api_key "$AUTH_A")"
AUTH_APIKEY_A="Authorization: ApiKey $API_KEY_A"
echo "Using API key to insert extra rows for $USER_A"
for i in $(seq -w 31 35); do
  id1="${USER_A}_ex1_${i}"
  id2="${USER_A}_ex2_${i}"
  curl -s -X POST "$BASE_URL/example1" \
    -H "$AUTH_APIKEY_A" -H "Content-Type: application/json" \
    -d "{\"field1\":\"$id1\",\"field2\":\"apikey_insert_${i}\"}" >/dev/null
  curl -s -X POST "$BASE_URL/example2" \
    -H "$AUTH_APIKEY_A" -H "Content-Type: application/json" \
    -d "{\"field1\":\"$id2\",\"field2\":\"API dept ${i}\"}" >/dev/null
done
 
E1_A_after="$(get_total_items "$AUTH_A" "example1")"
echo "$USER_A now sees example1: $E1_A_after items after API key inserts"
echo ""
 
# Optional cleanup for demo data created by alice and bob
if [ "$DELETE_AFTER_INSERT" = true ]; then
  echo "Cleanup enabled"

  # delete alice data
  ids_ex1_a=$(curl -s "$BASE_URL/example1?page_size=1000" -H "$AUTH_A" | jq -r '.data[].field1')
  ids_ex2_a=$(curl -s "$BASE_URL/example2?page_size=1000" -H "$AUTH_A" | jq -r '.data[].field1')
  rel_ids_a=$(curl -s "$BASE_URL/exampleRelational?page_size=1000" -H "$AUTH_A" \
    | jq -r '.data[] | "\(.example1_field1)|\(.example2_field1)"')
  for id in $ids_ex1_a; do curl -s -X DELETE "$BASE_URL/example1/$id" -H "$AUTH_A" >/dev/null; done
  for id in $ids_ex2_a; do curl -s -X DELETE "$BASE_URL/example2/$id" -H "$AUTH_A" >/dev/null; done
  while IFS='|' read -r a b; do
    curl -s -X DELETE "$BASE_URL/exampleRelational/${a}-${b}" -H "$AUTH_A" >/dev/null || true
  done <<< "$rel_ids_a"

  # delete bob data
  ids_ex1_b=$(curl -s "$BASE_URL/example1?page_size=1000" -H "$AUTH_B" | jq -r '.data[].field1')
  ids_ex2_b=$(curl -s "$BASE_URL/example2?page_size=1000" -H "$AUTH_B" | jq -r '.data[].field1')
  rel_ids_b=$(curl -s "$BASE_URL/exampleRelational?page_size=1000" -H "$AUTH_B" \
    | jq -r '.data[] | "\(.example1_field1)|\(.example2_field1)"')
  for id in $ids_ex1_b; do curl -s -X DELETE "$BASE_URL/example1/$id" -H "$AUTH_B" >/dev/null; done
  for id in $ids_ex2_b; do curl -s -X DELETE "$BASE_URL/example2/$id" -H "$AUTH_B" >/dev/null; done
  while IFS='|' read -r a b; do
    curl -s -X DELETE "$BASE_URL/exampleRelational/${a}-${b}" -H "$AUTH_B" >/dev/null || true
  done <<< "$rel_ids_b"

  echo "Cleanup complete"
fi

echo "Done"
