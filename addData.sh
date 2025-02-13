JWT=`curl -X 'POST' 'http://127.0.0.1:8088/login'  -d '{
 "password": "SuperSecurePassword",
 "username": "admin"
}' | jq .token | tr -d '"'`

# example1
curl -X 'POST' 'http://127.0.0.1:8088/example1' -H 'Authorization: '$JWT'' -d '{
 "field1": "field1Value1",
 "field2": "field2Value1"
}'

curl -X 'POST' 'http://127.0.0.1:8088/example1' -H 'Authorization: '$JWT'' -d '{
 "field1": "field1Value2",
 "field2": "field2Value2"
}'

curl -X 'POST' 'http://127.0.0.1:8088/example1' -H 'Authorization: '$JWT'' -d '{
 "field1": "field1Value3",
 "field2": "field2Value3"
}'

curl -X 'GET' 'http://127.0.0.1:8088/example1' -H 'Authorization: '$JWT'' | jq

# DELETE
curl -X 'DELETE' 'http://127.0.0.1:8088/example1/field1Value1' -H 'Authorization: '$JWT'' 
curl -X 'DELETE' 'http://127.0.0.1:8088/example1/field1Value2' -H 'Authorization: '$JWT'' 
curl -X 'DELETE' 'http://127.0.0.1:8088/example1/field1Value3' -H 'Authorization: '$JWT'' 
curl -X 'DELETE' 'http://127.0.0.1:8088/example1/field1Value4' -H 'Authorization: '$JWT'' 

curl -X 'GET' 'http://127.0.0.1:8088/example1' -H 'Authorization: '$JWT'' | jq


# example2
curl -X 'POST' 'http://127.0.0.1:8088/example2' -H 'Authorization: '$JWT'' -d '{
 "field1": "field1Value1",
 "field2": "field2Value1"
}'

curl -X 'POST' 'http://127.0.0.1:8088/example2' -H 'Authorization: '$JWT'' -d '{
 "field1": "field1Value2",
 "field2": "field2Value2"
}'

curl -X 'POST' 'http://127.0.0.1:8088/example2' -H 'Authorization: '$JWT'' -d '{
 "field1": "field1Value3",
 "field2": "field2Value3"
}'

curl -X 'GET' 'http://127.0.0.1:8088/example2' -H 'Authorization: '$JWT'' | jq


# Relational
curl -X 'POST' 'http://127.0.0.1:8088/exampleRelational' -H 'Authorization: '$JWT'' -d '{
 "example1_field1": "field1Value1",
 "example2_field1": "field1Value2",
 "Field3": "test1"
}'

curl -X 'GET' 'http://127.0.0.1:8088/exampleRelational' -H 'Authorization: '$JWT'' | jq
