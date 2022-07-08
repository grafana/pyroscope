package firedb

import (
	"github.com/google/uuid"

	schemav1 "github.com/grafana/fire/pkg/firedb/schemas/v1"
)

type profilesHelper struct{}

func (*profilesHelper) key(s *schemav1.Profile) profilesKey {
	id := s.ID
	if id == uuid.Nil {
		id = uuid.New()
	}
	return profilesKey{
		ID: id,
	}

}

func (*profilesHelper) addToRewriter(r *rewriter, elemRewriter idConversionTable) {
	r.locations = elemRewriter
}

func (*profilesHelper) rewrite(r *rewriter, s *schemav1.Profile) error {

	for pos := range s.Comment {
		r.strings.rewrite(&s.Comment[pos])
	}

	r.strings.rewrite(&s.DropFrames)
	r.strings.rewrite(&s.KeepFrames)

	return nil
}

type profilesKey struct {
	ID uuid.UUID
}
