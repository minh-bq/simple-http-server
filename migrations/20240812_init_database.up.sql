CREATE TABLE IF NOT EXISTS user_balance
(
    id      TEXT,
    balance BIGINT NOT NULL,
    PRIMARY KEY(id)
);

CREATE TABLE IF NOT EXISTS user_axie
(
    id      TEXT,
    axie    BIGINT NOT NULL,
    PRIMARY KEY(id)
);
