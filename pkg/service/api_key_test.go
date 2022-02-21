package service_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pyroscope-io/pyroscope/pkg/model"
	"github.com/pyroscope-io/pyroscope/pkg/service"
)

var _ = Describe("APIKeyService", func() {
	s := new(testSuite)
	BeforeEach(s.BeforeEach)
	AfterEach(s.AfterEach)

	var svc service.APIKeyService
	BeforeEach(func() {
		svc = service.NewAPIKeyService(s.DB())
	})

	Describe("API key creation", func() {
		var (
			params = testCreateAPIKeyParams()
			apiKey model.APIKey
			key    string
			err    error
		)

		JustBeforeEach(func() {
			apiKey, key, err = svc.CreateAPIKey(context.Background(), params)
		})

		Context("when a new API key created", func() {
			It("does not return error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("should populate the fields correctly", func() {
				expectAPIKeyMatches(apiKey, params)
			})

			It("creates a valid secret", func() {
				id, secret, err := model.DecodeAPIKey(key)
				Expect(err).ToNot(HaveOccurred())
				Expect(id).To(Equal(apiKey.ID))
				Expect(apiKey.Verify(secret)).ToNot(HaveOccurred())
			})
		})

		Context("when API key name is already in use", func() {
			BeforeEach(func() {
				_, _, err = svc.CreateAPIKey(context.Background(), params)
				Expect(err).ToNot(HaveOccurred())
			})

			It("returns validation error", func() {
				Expect(model.IsValidationError(err)).To(BeTrue())
			})
		})

		Context("when parameters are invalid", func() {
			BeforeEach(func() {
				params = model.CreateAPIKeyParams{}
			})

			It("returns validation error", func() {
				Expect(model.IsValidationError(err)).To(BeTrue())
			})
		})
	})

	Describe("API key retrieval", func() {
		Context("when an existing API key is queried", func() {
			It("can be found by id", func() {
				params := testCreateAPIKeyParams()
				k, _, err := svc.CreateAPIKey(context.Background(), params)
				Expect(err).ToNot(HaveOccurred())
				actual, err := svc.FindAPIKeyByID(context.Background(), k.ID)
				Expect(err).ToNot(HaveOccurred())
				expectAPIKeyMatches(actual, params)
			})
		})

		Context("when a non-existing API key is queried", func() {
			It("returns ErrAPIKeyNotFound error of NotFoundError type", func() {
				_, err := svc.FindAPIKeyByID(context.Background(), 13)
				Expect(err).To(MatchError(model.ErrAPIKeyNotFound))
			})
		})
	})

	Describe("API keys retrieval", func() {
		var (
			params = []model.CreateAPIKeyParams{
				{Name: "key-a", Role: model.AdminRole},
				{Name: "key-b", Role: model.ReadOnlyRole},
			}
			apiKeys []model.APIKey
			err     error
		)

		JustBeforeEach(func() {
			apiKeys, err = svc.GetAllAPIKeys(context.Background())
		})

		Context("when all API keys are queried", func() {
			BeforeEach(func() {
				for _, user := range params {
					_, _, err = svc.CreateAPIKey(context.Background(), user)
					Expect(err).ToNot(HaveOccurred())
				}
			})

			It("does not return error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("returns all keys", func() {
				apiKeyA, err := svc.FindAPIKeyByName(context.Background(), params[0].Name)
				Expect(err).ToNot(HaveOccurred())
				apiKeyB, err := svc.FindAPIKeyByName(context.Background(), params[1].Name)
				Expect(err).ToNot(HaveOccurred())
				Expect(apiKeys).To(ConsistOf(apiKeyA, apiKeyB))
			})
		})

		Context("when no API keys exist", func() {
			It("returns no error", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(apiKeys).To(BeEmpty())
			})
		})
	})

	Describe("API key delete", func() {
		var (
			params = []model.CreateAPIKeyParams{
				{Name: "key-a", Role: model.AdminRole},
				{Name: "key-b", Role: model.ReadOnlyRole},
			}
			apiKeys []model.APIKey
			err     error
		)

		JustBeforeEach(func() {
			err = svc.DeleteAPIKeyByID(context.Background(), apiKeys[0].ID)
		})

		Context("when existing API key deleted", func() {
			BeforeEach(func() {
				apiKeys = apiKeys[:0]
				for _, p := range params {
					apiKey, _, err := svc.CreateAPIKey(context.Background(), p)
					Expect(err).ToNot(HaveOccurred())
					apiKeys = append(apiKeys, apiKey)
				}
			})

			It("does not return error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("removes API key from the database", func() {
				_, err = svc.FindAPIKeyByName(context.Background(), apiKeys[0].Name)
				Expect(err).To(MatchError(model.ErrAPIKeyNotFound))
			})

			It("does not affect other API keys", func() {
				_, err = svc.FindAPIKeyByName(context.Background(), apiKeys[1].Name)
				Expect(err).ToNot(HaveOccurred())
			})

			It("allows API key with the same name to be created", func() {
				_, _, err = svc.CreateAPIKey(context.Background(), params[0])
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when non-existing API key deleted", func() {
			It("does not return error", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})

func testCreateAPIKeyParams() model.CreateAPIKeyParams {
	expiresAt := time.Date(3000, 12, 10, 4, 14, 0, 0, time.UTC)
	return model.CreateAPIKeyParams{
		Name:      "johndoe",
		Role:      model.ReadOnlyRole,
		ExpiresAt: &expiresAt,
	}
}

func expectAPIKeyMatches(apiKey model.APIKey, params model.CreateAPIKeyParams) {
	Expect(apiKey.Name).To(Equal(params.Name))
	Expect(apiKey.Role).To(Equal(params.Role))
	Expect(apiKey.ExpiresAt).To(Equal(params.ExpiresAt))
	Expect(apiKey.CreatedAt).ToNot(BeZero())
	Expect(apiKey.LastSeenAt).To(BeZero())
}
