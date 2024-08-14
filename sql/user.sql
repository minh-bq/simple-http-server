-- name: GetUserBalance :one
SELECT balance FROM user_balance WHERE id = $1;

-- name: UpsertUserBalance :exec
INSERT INTO user_balance(id, balance) VALUES ($1, $2) ON CONFLICT (id)
    DO UPDATE SET balance = $2;

-- name: GetUserAxie :one
SELECT axie FROM user_axie WHERE id = $1;

-- name: UpsertUserAxie :exec
INSERT INTO user_axie(id, axie) VALUES ($1, $2) ON CONFLICT (id)
    DO UPDATE SET axie = $2;
