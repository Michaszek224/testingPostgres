# Simple test project

## Testing project for technologies such as:

- golang
- dockerfile
- dockercompose
- postgresql
- redis

## How to test:

### Clone repository
<code>git clone github.com/michaszek224/testingpostgres</code>

### Launch docker compose:
<code>docker compose up</code>

### Testing api result with httpie:

- Getting all data
<code>http :8080</code>

- Getting single data
<code>http :8080/"id"</code>

- Add new data
<code>http :8080 name="name"</code>

- Deleting data
<code>http delete :8080/"id"</code>

- Updating data
<code>http put :8080/"id" name="new name"</code>



## i know pushing .env file is not the best idea
