package service_test

import (
	"time"

	"github.com/golang-jwt/jwt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/service"
)

var _ = Describe("API key JWT encoding", func() {
	var (
		userName string
		userRole model.Role
		tokenTTL time.Duration
		svc      service.JWTTokenService

		token  *jwt.Token
		signed string
		err    error
		key    []byte
	)

	BeforeEach(func() {
		userName = "johndoe"
		userRole = model.AdminRole
		key = []byte("signing-key")
		tokenTTL = 0
	})

	JustBeforeEach(func() {
		svc = service.NewJWTTokenService(key, tokenTTL)
		token = svc.GenerateUserJWTToken(userName, userRole)
		signed, err = svc.Sign(token)
	})

	Context("when a new token is generated for a user", func() {
		It("does not return error", func() {
			Expect(err).ToNot(HaveOccurred())
		})

		It("produces a valid JWT token", func() {
			parsed, parseErr := svc.Parse(signed)
			Expect(parseErr).ToNot(HaveOccurred())
			Expect(parsed.Valid).To(BeTrue())
		})
	})

	Context("invalid JWT token", func() {
		Context("when an expired JWT token is parsed", func() {
			BeforeEach(func() {
				tokenTTL = time.Millisecond
			})
			It("returns error if token has expired", func() {
				time.Sleep(time.Second)
				_, err = svc.Parse(signed)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when a token with invalid signature is parsed", func() {
			It("returns error if its signature can not be verified", func() {
				svc = service.NewJWTTokenService([]byte("invalid"), tokenTTL)
				_, err = svc.Parse(signed)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("when a token is acquired with UserFromJWTToken", func() {
		It("creates a valid user token", func() {
			user, ok := svc.UserFromJWTToken(svc.GenerateUserJWTToken(userName, userRole))
			Expect(ok).To(BeTrue())
			Expect(user).To(Equal(model.TokenUser{
				Name: userName,
				Role: userRole,
			}))
		})

		It("returns false if user token can not be retrieved", func() {
			_, ok := svc.UserFromJWTToken(svc.GenerateUserJWTToken("", model.InvalidRole))
			Expect(ok).To(BeFalse())
		})
	})
})
