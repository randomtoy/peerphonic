package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
)

func TestCatalogRoundTripAndReplacement(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer catalog.Close()

	track := domain.Track{
		ID: "track-1", Title: "One", Artist: "Artist", ArtistID: "artist-1",
		Album: "Album", AlbumID: "album-1", AlbumArtist: "Artist",
		TrackNumber: 1, Year: 2026, Genre: "Rock", Duration: 3*time.Minute + 5*time.Second,
		Size: 42, BitRate: 900, Suffix: "flac", ContentType: "audio/flac", CoverArtID: "art-1",
	}
	source := domain.SourceRef{Provider: "local", Key: "Artist/Album/One.flac"}
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{{Track: track, Ref: source}}, nil); err != nil {
		t.Fatalf("ReplaceProviderTracks() error = %v", err)
	}

	got, err := catalog.Track(ctx, track.ID)
	if err != nil {
		t.Fatalf("Track() error = %v", err)
	}
	if got.Title != track.Title || got.Duration != track.Duration ||
		got.CoverArtID != track.CoverArtID || got.Genre != track.Genre {
		t.Fatalf("Track() = %#v, want %#v", got, track)
	}
	sources, err := catalog.Sources(ctx, track.ID)
	if err != nil || len(sources) != 1 || sources[0] != source {
		t.Fatalf("Sources() = %#v, %v", sources, err)
	}
	artists, err := catalog.Artists(ctx)
	if err != nil || len(artists) != 1 || artists[0].Name != "Artist" {
		t.Fatalf("Artists() = %#v, %v", artists, err)
	}
	if artists[0].AlbumCount != 1 {
		t.Fatalf("artist album count = %d, want 1", artists[0].AlbumCount)
	}
	allAlbums, err := catalog.Albums(ctx, ports.AlbumListQuery{Limit: 10})
	if err != nil || len(allAlbums) != 1 || allAlbums[0].ID != "album-1" {
		t.Fatalf("Albums() = %#v, %v", allAlbums, err)
	}
	if allAlbums[0].CoverArtID != "art-1" {
		t.Fatalf("album cover art ID = %q", allAlbums[0].CoverArtID)
	}
	if allAlbums[0].Genre != "Rock" {
		t.Fatalf("album genre = %q, want Rock", allAlbums[0].Genre)
	}
	genres, err := catalog.Genres(ctx)
	if err != nil || len(genres) != 1 || genres[0] != (domain.Genre{Name: "Rock", SongCount: 1, AlbumCount: 1}) {
		t.Fatalf("Genres() = %#v, %v", genres, err)
	}
	albums, err := catalog.AlbumsByArtist(ctx, "artist-1")
	if err != nil || len(albums) != 1 || albums[0].SongCount != 1 {
		t.Fatalf("AlbumsByArtist() = %#v, %v", albums, err)
	}
	tracks, err := catalog.TracksByAlbum(ctx, "album-1")
	if err != nil || len(tracks) != 1 || tracks[0].ID != track.ID {
		t.Fatalf("TracksByAlbum() = %#v, %v", tracks, err)
	}
	genreTracks, err := catalog.TracksByGenre(ctx, "rock", 0, 10)
	if err != nil || len(genreTracks) != 1 || genreTracks[0].ID != track.ID {
		t.Fatalf("TracksByGenre() = %#v, %v", genreTracks, err)
	}
	genreTracks, err = catalog.TracksByGenre(ctx, "Rock", 1, 10)
	if err != nil || len(genreTracks) != 0 {
		t.Fatalf("paged TracksByGenre() = %#v, %v", genreTracks, err)
	}
	randomTracks, err := catalog.RandomTracks(ctx, ports.RandomTracksQuery{
		Limit: 10, Genre: "rock", FromYear: 2020, ToYear: 2030,
	})
	if err != nil || len(randomTracks) != 1 || randomTracks[0].ID != track.ID {
		t.Fatalf("RandomTracks() = %#v, %v", randomTracks, err)
	}
	randomTracks, err = catalog.RandomTracks(ctx, ports.RandomTracksQuery{Limit: 10, ToYear: 2020})
	if err != nil || len(randomTracks) != 0 {
		t.Fatalf("filtered RandomTracks() = %#v, %v", randomTracks, err)
	}

	if err := catalog.ReplaceProviderTracks(ctx, "local", nil, nil); err != nil {
		t.Fatalf("empty ReplaceProviderTracks() error = %v", err)
	}
	_, err = catalog.Track(ctx, track.ID)
	if !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Track() after replacement error = %v, want ErrNotFound", err)
	}
}

func TestCatalogSavesSelectedTrackSourceIncrementally(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()

	first := domain.TrackSource{
		Track: domain.Track{
			ID: "remote-1", Title: "First", Artist: "Artist", ArtistID: "artist-1",
			Album: "Album", AlbumID: "album-1", AlbumArtist: "Artist", Suffix: "mp3",
		},
		Ref: domain.SourceRef{Provider: "remote", Key: "peer/first.mp3"},
	}
	second := domain.TrackSource{
		Track: domain.Track{
			ID: "remote-2", Title: "Second", Artist: "Artist", ArtistID: "artist-1",
			Album: "Album", AlbumID: "album-1", AlbumArtist: "Artist", Suffix: "flac",
		},
		Ref: domain.SourceRef{Provider: "remote", Key: "peer/second.flac"},
	}
	if err := catalog.SaveTrackSource(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveTrackSource(ctx, second); err != nil {
		t.Fatal(err)
	}

	first.Track.Title = "First (updated)"
	if err := catalog.SaveTrackSource(ctx, first); err != nil {
		t.Fatal(err)
	}
	tracks, err := catalog.TracksByAlbum(ctx, "album-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 2 || tracks[0].Title != "First (updated)" || tracks[1].Title != "Second" {
		t.Fatalf("TracksByAlbum() = %#v", tracks)
	}
	sources, err := catalog.Sources(ctx, first.Track.ID)
	if err != nil || len(sources) != 1 || sources[0] != first.Ref {
		t.Fatalf("Sources() = %#v, %v", sources, err)
	}
}

func TestCatalogSavesTrackSourceBatchAtomically(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	valid := domain.TrackSource{
		Track: domain.Track{ID: "remote-1", Title: "First", Artist: "Artist", ArtistID: "artist-1"},
		Ref:   domain.SourceRef{Provider: "remote", Key: "peer/first.mp3"},
	}
	invalid := domain.TrackSource{Track: domain.Track{ID: "remote-2"}}
	if err := catalog.SaveTrackSources(ctx, []domain.TrackSource{valid, invalid}); err == nil {
		t.Fatal("SaveTrackSources() error = nil")
	}
	if _, err := catalog.Track(ctx, valid.Track.ID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Track() error = %v, want ErrNotFound after rollback", err)
	}
}

func TestCatalogAlbumListOrderingAndYearRanges(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	items := []domain.TrackSource{
		{Track: domain.Track{
			ID: "track-b", Title: "Song B", Artist: "Zulu", ArtistID: "artist-z",
			Album: "Beta", AlbumID: "album-b", AlbumArtist: "Zulu", Year: 2000, Genre: "Rock",
		}, Ref: domain.SourceRef{Provider: "local", Key: "b.mp3"}, DiscoveredAt: time.Unix(1, 0)},
		{Track: domain.Track{
			ID: "track-a", Title: "Song A", Artist: "Yankee", ArtistID: "artist-y",
			Album: "Alpha", AlbumID: "album-a", AlbumArtist: "Yankee", Year: 2020, Genre: "Pop",
		}, Ref: domain.SourceRef{Provider: "local", Key: "a.mp3"}, DiscoveredAt: time.Unix(2, 0)},
		{Track: domain.Track{
			ID: "track-c", Title: "Song C", Artist: "Able", ArtistID: "artist-a",
			Album: "Charlie", AlbumID: "album-c", AlbumArtist: "Able", Year: 2010, Genre: "Rock",
		}, Ref: domain.SourceRef{Provider: "local", Key: "c.mp3"}, DiscoveredAt: time.Unix(3, 0)},
		{Track: domain.Track{
			ID: "track-a2", Title: "Song A2", Artist: "Yankee", ArtistID: "artist-y",
			Album: "Alpha", AlbumID: "album-a", AlbumArtist: "Yankee", Year: 2020, Genre: "Rock",
		}, Ref: domain.SourceRef{Provider: "local", Key: "a2.mp3"}, DiscoveredAt: time.Unix(2, 0)},
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", items, nil); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		query ports.AlbumListQuery
		want  []string
	}{
		{name: "album name page", query: ports.AlbumListQuery{
			Offset: 1, Limit: 1, Order: ports.AlbumOrderName,
		}, want: []string{"Beta"}},
		{name: "album artist", query: ports.AlbumListQuery{
			Limit: 3, Order: ports.AlbumOrderArtist,
		}, want: []string{"Charlie", "Alpha", "Beta"}},
		{name: "newest", query: ports.AlbumListQuery{
			Limit: 3, Order: ports.AlbumOrderNewest,
		}, want: []string{"Charlie", "Alpha", "Beta"}},
		{name: "year ascending", query: ports.AlbumListQuery{
			Limit: 3, Order: ports.AlbumOrderYearAsc, FromYear: 2005, ToYear: 2025,
		}, want: []string{"Charlie", "Alpha"}},
		{name: "year descending", query: ports.AlbumListQuery{
			Limit: 3, Order: ports.AlbumOrderYearDesc, FromYear: 2025, ToYear: 2005,
		}, want: []string{"Alpha", "Charlie"}},
		{name: "genre", query: ports.AlbumListQuery{
			Limit: 3, Order: ports.AlbumOrderName, Genre: "rock",
		}, want: []string{"Alpha", "Beta", "Charlie"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			albums, err := catalog.Albums(ctx, test.query)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(albums))
			for _, album := range albums {
				got = append(got, album.Name)
			}
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Fatalf("Albums() = %v, want %v", got, test.want)
			}
		})
	}
	items[0], items[2] = items[2], items[0]
	if err := catalog.ReplaceProviderTracks(ctx, "local", items, nil); err != nil {
		t.Fatal(err)
	}
	albums, err := catalog.Albums(ctx, ports.AlbumListQuery{Limit: 3, Order: ports.AlbumOrderNewest})
	if err != nil {
		t.Fatal(err)
	}
	if albums[0].Name != "Charlie" || albums[1].Name != "Alpha" || albums[2].Name != "Beta" {
		t.Fatalf("newest albums after rescan = %#v", albums)
	}
}

func TestCatalogReplacementIsAtomic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	original := domain.Track{
		ID: "original", Title: "Original", Artist: "Artist", ArtistID: "artist",
		Album: "Album", AlbumID: "album", AlbumArtist: "Artist",
	}
	originalSource := domain.TrackSource{
		Track: original, Ref: domain.SourceRef{Provider: "local", Key: "original.mp3"},
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{originalSource}, nil); err != nil {
		t.Fatal(err)
	}
	invalid := originalSource
	invalid.Track.ID = "invalid"
	invalid.Ref.Provider = "remote"
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{invalid}, nil); err == nil {
		t.Fatal("ReplaceProviderTracks() error = nil, want provider mismatch")
	}
	if _, err := catalog.Track(ctx, original.ID); err != nil {
		t.Fatalf("original track lost after rollback: %v", err)
	}
}

func TestCatalogRetainsAlbumAliasesAcrossMetadataChanges(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()

	source := domain.TrackSource{
		Track: domain.Track{
			ID: "track-1", Title: "Song", Artist: "Artist", ArtistID: "artist",
			Album: "First", AlbumID: "album-first", AlbumArtist: "Artist",
		},
		Ref: domain.SourceRef{Provider: "local", Key: "song.mp3"},
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{source}, nil); err != nil {
		t.Fatal(err)
	}
	source.Track.Album = "Second"
	source.Track.AlbumID = "album-second"
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{source}, nil); err != nil {
		t.Fatal(err)
	}
	source.Track.Album = "Third"
	source.Track.AlbumID = "album-third"
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{source}, nil); err != nil {
		t.Fatal(err)
	}
	for _, albumID := range []string{"album-first", "album-second", "album-third"} {
		tracks, err := catalog.TracksByAlbum(ctx, albumID)
		if err != nil || len(tracks) != 1 || tracks[0].AlbumID != "album-third" {
			t.Fatalf("TracksByAlbum(%q) = %#v, %v", albumID, tracks, err)
		}
	}
}

func TestCatalogUpdatesTrackMetadataWithoutReplacingSources(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()

	track := domain.Track{
		ID: "remote-track", Title: "01 Song", Artist: "Unknown", ArtistID: "artist-old",
		Album: "Folder", AlbumID: "album-old", AlbumArtist: "Unknown",
	}
	ref := domain.SourceRef{Provider: "torrent", Key: "hash/01 Song.mp3"}
	if err := catalog.ReplaceProviderTracks(ctx, "torrent", []domain.TrackSource{{
		Track: track, Ref: ref,
	}}, nil); err != nil {
		t.Fatal(err)
	}
	track.Title = "Tagged Song"
	track.Artist = "Tagged Artist"
	track.ArtistID = "artist-new"
	track.Album = "Tagged Album"
	track.AlbumID = "album-new"
	track.AlbumArtist = "Tagged Artist"
	track.AlbumArtistID = "artist-new"
	track.Duration = 3 * time.Minute
	track.BitRate = 320
	track.Genre = "Electronic"
	if err := catalog.UpdateTrack(ctx, track); err != nil {
		t.Fatal(err)
	}

	updated, err := catalog.Track(ctx, track.ID)
	if err != nil || updated.Title != "Tagged Song" || updated.Duration != 3*time.Minute ||
		updated.BitRate != 320 || updated.Genre != "Electronic" {
		t.Fatalf("Track() = %#v, %v", updated, err)
	}
	sources, err := catalog.Sources(ctx, track.ID)
	if err != nil || len(sources) != 1 || sources[0] != ref {
		t.Fatalf("Sources() = %#v, %v", sources, err)
	}
	for _, albumID := range []string{"album-old", "album-new"} {
		tracks, err := catalog.TracksByAlbum(ctx, albumID)
		if err != nil || len(tracks) != 1 || tracks[0].AlbumID != "album-new" {
			t.Fatalf("TracksByAlbum(%q) = %#v, %v", albumID, tracks, err)
		}
	}
	if err := catalog.UpdateTrack(ctx, domain.Track{ID: "missing"}); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("UpdateTrack(missing) error = %v, want ErrNotFound", err)
	}
}

func TestCatalogKeepsTrackUntilItsLastSourceIsRemoved(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()

	track := domain.Track{
		ID: "shared-track", Title: "Shared", Artist: "Artist", ArtistID: "artist",
		Album: "Album", AlbumID: "album", AlbumArtist: "Artist",
	}
	localSource := domain.TrackSource{
		Track: track, Ref: domain.SourceRef{Provider: "local", Key: "shared.mp3"},
	}
	remoteSource := domain.TrackSource{
		Track: track, Ref: domain.SourceRef{Provider: "remote", Key: "peer/shared.mp3"},
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", []domain.TrackSource{localSource}, nil); err != nil {
		t.Fatal(err)
	}
	if err := catalog.ReplaceProviderTracks(ctx, "remote", []domain.TrackSource{remoteSource}, nil); err != nil {
		t.Fatal(err)
	}
	sources, err := catalog.Sources(ctx, track.ID)
	if err != nil || len(sources) != 2 {
		t.Fatalf("Sources() = %#v, %v; want two sources", sources, err)
	}

	if err := catalog.ReplaceProviderTracks(ctx, "local", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Track(ctx, track.ID); err != nil {
		t.Fatalf("Track() after local removal error = %v", err)
	}
	sources, err = catalog.Sources(ctx, track.ID)
	if err != nil || len(sources) != 1 || sources[0] != remoteSource.Ref {
		t.Fatalf("Sources() after local removal = %#v, %v", sources, err)
	}

	if err := catalog.ReplaceProviderTracks(ctx, "remote", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Track(ctx, track.ID); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("Track() after final removal error = %v, want ErrNotFound", err)
	}
}

func TestCatalogSearchIsUnicodeAwareAndPagedIndependently(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	catalog, err := Open(ctx, filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	tracks := []domain.TrackSource{
		{
			Track: domain.Track{
				ID: "track-crow", Title: "Пластилиновая Ворона", Artist: "Черная Метка",
				ArtistID: "artist-black", Album: "Hard Covers", AlbumID: "album-hard",
				AlbumArtist: "Черная Метка",
			},
			Ref: domain.SourceRef{Provider: "local", Key: "crow.mp3"},
		},
		{
			Track: domain.Track{
				ID: "track-song", Title: "Another Song", Artist: "Other Artist",
				ArtistID: "artist-other", Album: "Other Album", AlbumID: "album-other",
				AlbumArtist: "Other Artist",
			},
			Ref: domain.SourceRef{Provider: "local", Key: "song.mp3"},
		},
	}
	if err := catalog.ReplaceProviderTracks(ctx, "local", tracks, nil); err != nil {
		t.Fatal(err)
	}

	result, err := catalog.Search(ctx, ports.CatalogSearch{
		Text: "черная", ArtistCount: 10, AlbumCount: 10, SongCount: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artists) != 1 || result.Artists[0].ID != "artist-black" {
		t.Fatalf("artists = %#v", result.Artists)
	}
	if len(result.Albums) != 1 || result.Albums[0].ID != "album-hard" {
		t.Fatalf("albums = %#v", result.Albums)
	}
	if len(result.Songs) != 1 || result.Songs[0].ID != "track-crow" {
		t.Fatalf("songs = %#v", result.Songs)
	}

	result, err = catalog.Search(ctx, ports.CatalogSearch{
		Text: "", ArtistOffset: 1, ArtistCount: 1, AlbumCount: 0, SongOffset: 1, SongCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artists) != 1 || len(result.Albums) != 0 || len(result.Songs) != 1 {
		t.Fatalf("paged result = %#v", result)
	}
}
