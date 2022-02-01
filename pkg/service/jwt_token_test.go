package service_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/service"
)

var _ = Describe("API key JWT encoding", func() {
	var (
		p   model.CreateAPIKeyParams
		svc service.JWTTokenService
	)

	BeforeEach(func() {
		p = model.CreateAPIKeyParams{
			Name: "api_key_name",
			Role: model.AdminRole,
		}

		svc = service.NewJWTTokenService([]byte("signing-key"), 0)
	})

	Context("when a new token is generated for an API key", func() {
		It("produces a valid JWT token", func() {
			t := svc.GenerateAPIKeyToken(p)
			signed, signature, err := svc.Sign(t)
			Expect(err).ToNot(HaveOccurred())
			parsed, parseErr := svc.Parse(signed)
			Expect(parseErr).ToNot(HaveOccurred())
			Expect(parsed.Signature).To(Equal(signature))
			Expect(parsed.Valid).To(BeTrue())
		})
	})

	Context("when an invalid JWT token is parsed", func() {
		It("returns error if token has expired", func() {
			exp := time.Now().Add(time.Second * -1)
			p.ExpiresAt = &exp
			signed, _, err := svc.Sign(svc.GenerateAPIKeyToken(p))
			Expect(err).ToNot(HaveOccurred())
			_, err = svc.Parse(signed)
			Expect(err).To(HaveOccurred())
		})

		It("returns error if its signature can not be verified", func() {
			signed, _, err := svc.Sign(svc.GenerateAPIKeyToken(p))
			Expect(err).ToNot(HaveOccurred())
			svc = service.NewJWTTokenService([]byte("invalid"), 0)
			_, err = svc.Parse(signed)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when a token is acquired with APIKeyTokenFromJWTToken", func() {
		It("creates a valid API key if the token is valid", func() {
			apiKey, ok := svc.APIKeyFromJWTToken(svc.GenerateAPIKeyToken(p))
			Expect(ok).To(BeTrue())
			Expect(apiKey).To(Equal(model.APIKeyToken{
				Name: p.Name,
				Role: p.Role,
			}))
		})

		It("returns false if API key can not be retrieved", func() {
			p = model.CreateAPIKeyParams{}
			_, ok := svc.APIKeyFromJWTToken(svc.GenerateAPIKeyToken(p))
			Expect(ok).To(BeFalse())
		})
	})
})
