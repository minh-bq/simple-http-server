package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"simple-http-server/db"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

type UserIdContextKeyType string

const (
	DSN                                   = "postgres://localhost:5432/store?user=store&password=123456&sslmode=disable"
	UserIdContextKey UserIdContextKeyType = "UserIdContext"
)

var shutdownTimeout = 30 * time.Second

func getUserIdFromCookie(req *http.Request) (string, error) {
	cookie, err := req.Cookie("userId")
	if err != nil {
		return "", err
	}

	return cookie.Value, nil
}

// mock authentication, please ignore the authentication part
func mockAuth(database *sql.DB, handler func(*sql.DB) http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {

		userId, err := getUserIdFromCookie(request)
		if err != nil {
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(request.Context(), UserIdContextKey, userId)
		handler(database).ServeHTTP(response, request.WithContext(ctx))
	})
}

type buyAxieRequest struct {
	Token int64 `json:"token"`
}

func buyAxie(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		ctx := request.Context()
		userId, ok := ctx.Value(UserIdContextKey).(string)
		if !ok {
			slog.Error("Missing user id in buyAxie")
			http.Error(response, "missing userId", http.StatusUnauthorized)
			return
		}

		defer request.Body.Close()

		var buyRequest buyAxieRequest
		if err := json.NewDecoder(request.Body).Decode(&buyRequest); err != nil {
			http.Error(response, "malformed request", http.StatusBadRequest)
			return
		}

		query := db.New(database)
		balance, err := query.GetUserBalance(context.Background(), userId)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(response, "insufficient balance", http.StatusBadRequest)
			return
		} else if err != nil {
			http.Error(response, "", http.StatusInternalServerError)
			return
		}

		if buyRequest.Token > balance {
			http.Error(response, "insufficient balance", http.StatusBadRequest)
			return
		}

		axie, err := query.GetUserAxie(context.Background(), userId)
		if errors.Is(err, sql.ErrNoRows) {
			axie = 0
		} else if err != nil {
			http.Error(response, "", http.StatusInternalServerError)
			return
		}

		tx, err := database.Begin()
		if err != nil {
			http.Error(response, "", http.StatusInternalServerError)
			return
		}

		defer tx.Rollback()
		query = query.WithTx(tx)

		err = query.UpsertUserAxie(
			context.Background(),
			db.UpsertUserAxieParams{ID: userId, Axie: axie + buyRequest.Token},
		)
		if err != nil {
			http.Error(response, "", http.StatusInternalServerError)
			return
		}

		err = query.UpsertUserBalance(
			context.Background(),
			db.UpsertUserBalanceParams{ID: userId, Balance: balance - buyRequest.Token},
		)
		if err != nil {
			http.Error(response, "", http.StatusInternalServerError)
			return
		}

		tx.Commit()
	})
}

func migration(database *sql.DB) error {
	driver, err := postgres.WithInstance(database, &postgres.Config{})
	if err != nil {
		slog.Error("Failed to create driver", "err", err)
		return err
	}

	m, err := migrate.NewWithDatabaseInstance("file://migrations", "postgres", driver)
	if err != nil {
		slog.Error("Failed to read migrations", "err", err)
		return err
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		slog.Error("Failed to run migrations", "err", err)
		return err
	}

	slog.Info("Finish migrations")
	return nil
}

func main() {
	database, err := sql.Open("postgres", DSN)
	if err != nil {
		slog.Error("Failed to connect to database", "err", err)
		os.Exit(1)
	}

	if err := migration(database); err != nil {
		os.Exit(1)
	}

	m := http.NewServeMux()
	m.Handle("POST /buy-axie", mockAuth(database, buyAxie))

	server := http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: m,
	}

	shutdown := make(chan error, 1)
	go func() {
		sig := make(chan os.Signal, 1)

		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		shutdown <- server.Shutdown(ctx)
	}()

	slog.Info("Listening on", "addr", server.Addr)
	server.ListenAndServe()
	if err := <-shutdown; err != nil {
		slog.Error("Failed to gracefully shutdown", "err", err)
		os.Exit(1)
	}
	slog.Info("Shutdown")
}
