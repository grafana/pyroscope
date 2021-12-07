package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/internal/model"
	"github.com/pyroscope-io/pyroscope/pkg/sqlstore"
)

func TestExample(t *testing.T) {
	db, err := sqlstore.Open(&sqlstore.Config{
		Logger: nil,
		Type:   "sqlite3",
		URL:    "/Users/kolesnikovae/Documents/src/pyroscope/out/test.db",
	})

	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	s := NewUserService(db.DB())
	fullName := "John Doe"
	user, err := s.CreateUser(ctx, model.CreateUserParams{
		FullName: &fullName,
		Email:    "jonny@example.com",
		Role:     model.ViewerRole,
		Password: []byte("qwerty"),
	})
	fmt.Println("CreateUser", err, user)
	if err != nil {
		return
	}

	time.Sleep(time.Second)
	role := model.AdminRole
	user, err = s.UpdateUserByID(ctx, user.ID, model.UpdateUserParams{
		FullName: nil,
		Email:    nil,
		Role:     &(role),
	})
	fmt.Println("UpdateUser", err, user)
	if err != nil {
		return
	}

	err = s.DeleteUserByID(ctx, user.ID)
	fmt.Println("DeleteUserByID", err, user)
	if err != nil {
		return
	}
}
