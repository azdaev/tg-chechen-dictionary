package business

import (
	"chetoru/internal/cache"
	"chetoru/internal/models"
	"chetoru/internal/repository"
	"context"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
)

type brokenDictRepo struct {
	recordingDictRepo
}

func (r *brokenDictRepo) FindTranslationPairs(context.Context, string, int) ([]models.TranslationPairs, error) {
	return nil, errors.New("database is locked")
}

func (r *brokenDictRepo) FindTranslationPairsByFolded(context.Context, string, int) ([]models.TranslationPairs, error) {
	return nil, errors.New("database is locked")
}

// The embedded fake posts every insert to a channel a test must drain; these
// tests care about the read path, so writes are dropped instead.
func (r *brokenDictRepo) InsertTranslationPair(context.Context, repository.TranslationPair) (int64, bool, error) {
	return 0, false, nil
}

// A layer that could not read is not a layer that found nothing. The local
// lookups swallowed their errors and returned an empty slice, so a database
// hiccup was indistinguishable from absence: dosham legitimately holding
// nothing under that spelling then pinned «нет перевода» in the cache for the
// full negative TTL, and the word went into the coverage report as a gap.
func TestTranslate_StorageFailureIsNotAbsence(t *testing.T) {
	probe := &doshamProbe{primaries: map[string]bool{"гӏала": true}}
	probe.start(t)

	b := &Business{log: logrus.New(), cache: cache.NewCache("127.0.0.1:1", ""), dictRepo: &brokenDictRepo{}}
	b.SetFoldedReady()

	got, _, err := b.TranslateResolved("гӏала")
	if err == nil {
		t.Fatalf("got %d pairs and no error; an unreadable dictionary was reported as a missing word", len(got))
	}
	if len(got) != 0 {
		t.Errorf("got %+v alongside the error", got)
	}
}

// The same failure must stay silent when something else answered: a broken
// folded lookup does not make a found translation an outage.
func TestTranslate_StorageFailureIsQuietWhenTheAPIAnswers(t *testing.T) {
	probe := &doshamProbe{
		primaries: map[string]bool{"дом": true},
		entries:   map[string]string{"дом": "Дом"},
	}
	probe.start(t)

	b := &Business{log: logrus.New(), cache: cache.NewCache("127.0.0.1:1", ""), dictRepo: &brokenDictRepo{}}
	b.SetFoldedReady()

	got, _, err := b.TranslateResolved("дом")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("the API answer was thrown away with the storage error")
	}
	b.WaitBackground()
}
