// Copyright 2026 The eblitui Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package artwork reads a core's artwork index and decides which local
// artwork files to download. The index lists, per game ID, the
// images the artwork site serves: their type, the values that tell images
// of one type apart (disc number, region, variant), path relative to the
// index, size, and hashes. The index is used only when its signature file
// holds a valid signature by one of the core's artwork keys, and a
// downloaded image is kept only when its size and SHA-256 match the signed
// index. A game's images are stored in its own directory under names built
// from the entry's values, never from its path:
// <type>[_<disc>][_<region>][_<variant>].<ext>.
package artwork

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	_ "image/jpeg"
	_ "image/png"

	"github.com/user-none/eblitui/desktop/netutil"
	"github.com/user-none/eblitui/desktop/signing"
	"github.com/user-none/eblitui/desktop/storage"
	_ "golang.org/x/image/webp"
)

// IndexFile is the index's name, relative to the artwork base URL.
const IndexFile = "index.json"

// SignatureFile is the name of the index's signature file, relative to the
// artwork base URL. See the signing package for its format.
const SignatureFile = "index.sig"

// IndexVersion is the index format this package reads.
const IndexVersion = 1

// MaxIndexSize is the largest index accepted: 16 MiB.
const MaxIndexSize = 16 * 1024 * 1024

// MaxImageSize is the largest image accepted: 10 MiB.
const MaxImageSize = 10 * 1024 * 1024

// maxRedirects is the most redirects followed for one request.
const maxRedirects = 10

// imageTypes are the image types this build downloads and shows, in display
// order. Entries of any other type are ignored.
var imageTypes = []string{"boxart", "boxback", "disc", "flyer", "poster"}

// typeFields are the values an image type is told apart by
var typeFields = map[string]struct{ disc, region, variant bool }{
	"boxart":  {region: true},
	"boxback": {region: true},
	"disc":    {disc: true, region: true},
	"flyer":   {variant: true},
	"poster":  {variant: true},
}

// typeCaptions name the image types on screen
var typeCaptions = map[string]string{
	"boxart":  "Front",
	"boxback": "Back",
	"disc":    "Disc",
	"flyer":   "Flyer",
	"poster":  "Poster",
}

// regionOrder is the order regional versions are shown and chosen in:
// no region first, then these, then any other region
var regionOrder = []string{"", "us", "jp", "eu"}

// regionNames name the regions shown by name; any other is "Other"
var regionNames = map[string]string{"us": "USA", "jp": "Japan", "eu": "Europe"}

// httpClient fetches the index and images. It follows redirects only to
// HTTPS URLs and uses the default transport, which verifies server
// certificates against the system's trusted roots.
var httpClient = &http.Client{
	Timeout: netutil.HTTPTimeout,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errors.New("redirect to a non-HTTPS URL")
		}
		if len(via) >= maxRedirects {
			return errors.New("too many redirects")
		}
		return nil
	},
}

// legacyBoxArt is the box art file name used before the artwork index.
// It is shown when a game has no boxart.webp yet, and removed by Clean
// Library once boxart.webp is present.
const legacyBoxArt = "boxart.png"

// extensions are the image file extensions accepted from the index.
var extensions = map[string]bool{".webp": true, ".png": true, ".jpg": true}

// MaxImageDimension is the largest width or height of an artwork image
// that is decoded.
const MaxImageDimension = 4096

var (
	crcPattern     = regexp.MustCompile(`^[0-9a-f]{8}$`)
	sha256Pattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	regionPattern  = regexp.MustCompile(`^[a-z]{2,}$`)
	variantPattern = regexp.MustCompile(`^[a-z]$`)

	// Local file name stems, per type
	boxStem   = regexp.MustCompile(`^(boxart|boxback)(?:_([a-z]{2,}))?$`)
	discStem  = regexp.MustCompile(`^disc(?:_([1-9][0-9]*))?(?:_([a-z]{2,}))?$`)
	flyerStem = regexp.MustCompile(`^(flyer|poster)(?:_([a-z]))?$`)
)

// File is one image in the index. Disc, Region and Variant tell images of
// one type apart; each is set only on the types it applies to (see
// typeFields) and only when the game has more than one such image.
type File struct {
	Type    string `json:"type"`
	Path    string `json:"path"`
	Disc    int    `json:"disc,omitempty"`    // disc number, 1 and up
	Region  string `json:"region,omitempty"`  // lower case letters, such as "us"
	Variant string `json:"variant,omitempty"` // one lower case letter
	Size    int64  `json:"size"`
	CRC32   string `json:"crc32"`
	SHA256  string `json:"sha256"`
}

// Index is a parsed artwork index.
type Index struct {
	Version int               `json:"version"`
	Games   map[string][]File `json:"games"`
}

// Parse decodes an index and checks its version and every entry. An index
// with an unknown version or any invalid entry is rejected as a whole.
func Parse(data []byte) (*Index, error) {
	if len(data) > MaxIndexSize {
		return nil, fmt.Errorf("artwork index exceeds %d bytes", MaxIndexSize)
	}
	var ix Index
	if err := json.Unmarshal(data, &ix); err != nil {
		return nil, fmt.Errorf("artwork index: %w", err)
	}
	if ix.Version != IndexVersion {
		return nil, fmt.Errorf("artwork index version %d is not supported", ix.Version)
	}
	for gameID, files := range ix.Games {
		for _, f := range files {
			if err := checkFile(f); err != nil {
				return nil, fmt.Errorf("artwork index entry for %s: %w", gameID, err)
			}
		}
	}
	return &ix, nil
}

// checkFile validates one index entry.
func checkFile(f File) error {
	if f.Type == "" || strings.ContainsAny(f.Type, `./\`) {
		return fmt.Errorf("invalid type %q", f.Type)
	}
	if !ValidPath(f.Path) {
		return fmt.Errorf("invalid path %q", f.Path)
	}
	if !extensions[strings.ToLower(path.Ext(f.Path))] {
		return fmt.Errorf("unsupported file type %q", f.Path)
	}
	if f.Size <= 0 || f.Size > MaxImageSize {
		return fmt.Errorf("invalid size %d for %q", f.Size, f.Path)
	}
	if !crcPattern.MatchString(f.CRC32) {
		return fmt.Errorf("invalid crc32 %q for %q", f.CRC32, f.Path)
	}
	if !sha256Pattern.MatchString(f.SHA256) {
		return fmt.Errorf("invalid sha256 %q for %q", f.SHA256, f.Path)
	}
	if f.Disc < 0 {
		return fmt.Errorf("invalid disc %d for %q", f.Disc, f.Path)
	}
	fields, known := typeFields[f.Type]
	if !known {
		return nil
	}
	if f.Disc != 0 && !fields.disc {
		return fmt.Errorf("invalid disc %d for %q", f.Disc, f.Path)
	}
	if f.Region != "" && (!fields.region || !regionPattern.MatchString(f.Region)) {
		return fmt.Errorf("invalid region %q for %q", f.Region, f.Path)
	}
	if f.Variant != "" && (!fields.variant || !variantPattern.MatchString(f.Variant)) {
		return fmt.Errorf("invalid variant %q for %q", f.Variant, f.Path)
	}
	return nil
}

// ValidPath reports whether p is a relative path of plain segments: no
// leading slash, no backslashes or control characters, and no empty, "."
// or ".." segments.
func ValidPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.ContainsRune(p, '\\') {
		return false
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// FileURL returns the URL of an index path under baseURL, which must be an
// absolute HTTPS URL with a host. The path's segments are escaped, so names
// with spaces are fetched correctly.
func FileURL(baseURL, p string) (string, error) {
	if !ValidPath(p) {
		return "", fmt.Errorf("invalid artwork path %q", p)
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid artwork URL %q: %w", baseURL, err)
	}
	if u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("artwork URL %q is not an HTTPS URL", baseURL)
	}
	return u.JoinPath(strings.Split(p, "/")...).String(), nil
}

// FetchIndex downloads the index and its signature file under baseURL,
// checks that the signature file holds a valid signature of the index by
// one of keys, and only then parses the index. Without keys, or when the
// signature does not verify, no index is returned.
func FetchIndex(baseURL string, keys signing.Keys) (*Index, error) {
	if keys.Empty() {
		return nil, errors.New("no keys to verify the artwork index")
	}
	sigURL, err := FileURL(baseURL, SignatureFile)
	if err != nil {
		return nil, err
	}
	indexURL, err := FileURL(baseURL, IndexFile)
	if err != nil {
		return nil, err
	}
	sig, err := netutil.Download(httpClient, sigURL, signing.MaxSignatureSize)
	if err != nil {
		return nil, fmt.Errorf("artwork index signature: %w", err)
	}
	data, err := netutil.Download(httpClient, indexURL, MaxIndexSize)
	if err != nil {
		return nil, err
	}
	if !signing.Verify(data, sig, keys) {
		return nil, errors.New("artwork index signature does not verify")
	}
	return Parse(data)
}

// Download fetches f from baseURL, checks it against the index entry, and
// writes it atomically to the game directory dir under LocalName. Nothing
// is written when the download fails or does not match.
func Download(baseURL, dir string, f File) error {
	u, err := FileURL(baseURL, f.Path)
	if err != nil {
		return err
	}
	data, err := netutil.Download(httpClient, u, MaxImageSize)
	if err != nil {
		return err
	}
	if err := Verify(data, f); err != nil {
		return err
	}
	return storage.AtomicWriteFile(filepath.Join(dir, LocalName(f)), data)
}

// Files returns a game's index entries whose type is one of types, in index
// order. Types this build does not know are never returned.
func (ix *Index) Files(gameID string, types []string) []File {
	if ix == nil {
		return nil
	}
	var out []File
	for _, f := range ix.Games[gameID] {
		if _, known := typeFields[f.Type]; known && slices.Contains(types, f.Type) {
			out = append(out, f)
		}
	}
	return out
}

// DownloadTypes returns the image types a scan downloads for an artwork
// download setting (storage.ArtworkDownload*): the front cover, the
// covers, or every type this build knows. Any other value downloads every
// type.
func DownloadTypes(setting string) []string {
	switch setting {
	case storage.ArtworkDownloadFront:
		return []string{"boxart"}
	case storage.ArtworkDownloadCovers:
		return []string{"boxart", "boxback"}
	default:
		return slices.Clone(imageTypes)
	}
}

// LocalName returns the file name an image is stored under in its game's
// directory, built from the entry's values and its path's extension:
// <type>[_<disc>][_<region>][_<variant>].<ext>, such as "disc_2_eu.webp".
// Parse has checked the values, so the name is a plain file name.
func LocalName(f File) string {
	name := f.Type
	if f.Disc > 0 {
		name += "_" + strconv.Itoa(f.Disc)
	}
	if f.Region != "" {
		name += "_" + f.Region
	}
	if f.Variant != "" {
		name += "_" + f.Variant
	}
	return name + strings.ToLower(path.Ext(f.Path))
}

// LocalFile is an artwork image in a game's directory, with the values its
// name carries
type LocalFile struct {
	Path    string
	Type    string
	Disc    int
	Region  string
	Variant string
}

// LocalFiles returns the artwork images in the game directory dir whose
// names follow LocalName for a known type, in display order: by type
// (front, back, disc, flyer, poster), then disc number, then region (none,
// USA, Japan, Europe, then any other by code), then variant. When two
// files carry the same values, the .webp one is used. Other files are
// ignored. A missing or empty directory gives nil.
func LocalFiles(dir string) []LocalFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	type key struct {
		typ, region, variant string
		disc                 int
	}
	found := map[key]LocalFile{}
	for _, e := range entries {
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if strings.HasPrefix(name, ".") || !e.Type().IsRegular() || !extensions[ext] {
			continue
		}
		f, ok := parseLocalName(strings.TrimSuffix(name, filepath.Ext(name)))
		if !ok {
			continue
		}
		f.Path = filepath.Join(dir, name)
		k := key{f.Type, f.Region, f.Variant, f.Disc}
		if prev, dup := found[k]; dup && strings.ToLower(filepath.Ext(prev.Path)) == ".webp" {
			continue
		}
		found[k] = f
	}
	if len(found) == 0 {
		return nil
	}
	out := make([]LocalFile, 0, len(found))
	for _, f := range found {
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b LocalFile) int {
		if c := cmp.Compare(slices.Index(imageTypes, a.Type), slices.Index(imageTypes, b.Type)); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Disc, b.Disc); c != 0 {
			return c
		}
		if c := cmp.Compare(regionRank(a.Region), regionRank(b.Region)); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Region, b.Region); c != 0 {
			return c
		}
		return cmp.Compare(a.Variant, b.Variant)
	})
	return out
}

// parseLocalName reads the values from a local file name without its
// extension. ok is false when the name does not follow LocalName for a
// known type.
func parseLocalName(stem string) (f LocalFile, ok bool) {
	if m := boxStem.FindStringSubmatch(stem); m != nil {
		return LocalFile{Type: m[1], Region: m[2]}, true
	}
	if m := discStem.FindStringSubmatch(stem); m != nil {
		f = LocalFile{Type: "disc", Region: m[2]}
		if m[1] != "" {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				return LocalFile{}, false
			}
			f.Disc = n
		}
		return f, true
	}
	if m := flyerStem.FindStringSubmatch(stem); m != nil {
		return LocalFile{Type: m[1], Variant: m[2]}, true
	}
	return LocalFile{}, false
}

// regionRank orders regions: none, USA, Japan, Europe, then any other
func regionRank(region string) int {
	if i := slices.Index(regionOrder, region); i >= 0 {
		return i
	}
	return len(regionOrder)
}

// Caption returns the on-screen name of a local image, such as "Front",
// "Disc 2 (USA)" or "Flyer B". A region other than USA, Japan and Europe
// is shown as "Other".
func Caption(f LocalFile) string {
	s := typeCaptions[f.Type]
	if f.Disc > 0 {
		s += " " + strconv.Itoa(f.Disc)
	}
	if f.Variant != "" {
		s += " " + strings.ToUpper(f.Variant)
	}
	if f.Region != "" {
		name, ok := regionNames[f.Region]
		if !ok {
			name = "Other"
		}
		s += " (" + name + ")"
	}
	return s
}

// NeedsDownload reports whether the game directory dir lacks f or holds a
// copy whose size or CRC-32 differs from the index.
func NeedsDownload(dir string, f File) bool {
	p := filepath.Join(dir, LocalName(f))
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() || info.Size() != f.Size {
		return true
	}
	sum, err := fileCRC32(p)
	return err != nil || sum != f.CRC32
}

// Verify checks downloaded image data against its index entry by size and
// SHA-256. CRC-32 is used only to notice changed local files; it does not
// show a download is the vetted image.
func Verify(data []byte, f File) error {
	if int64(len(data)) != f.Size {
		return fmt.Errorf("%s: got %d bytes, want %d", f.Path, len(data), f.Size)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != f.SHA256 {
		return fmt.Errorf("%s: sha256 %s, want %s", f.Path, got, f.SHA256)
	}
	return nil
}

// Load decodes the artwork image file at path. The dimensions are read
// from the image header first and an image larger than MaxImageDimension
// on either side is refused before it is decoded, so the memory a file can
// take is bounded by the dimension limit, whatever the file size.
func Load(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, _, err := readSize(f); err != nil {
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(f)
	return img, err
}

// Size returns the width and height of the artwork image file at path,
// read from its header alone, so it is cheap enough to call before
// deciding on a layout. It returns an error for a file Load refuses on its
// dimensions.
func Size(path string) (width, height int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	return readSize(f)
}

// readSize reads an image's dimensions from its header and checks them
// against MaxImageDimension
func readSize(r io.Reader) (width, height int, err error) {
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return 0, 0, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxImageDimension || cfg.Height > MaxImageDimension {
		return 0, 0, fmt.Errorf("image is %dx%d, larger than %dx%d", cfg.Width, cfg.Height, MaxImageDimension, MaxImageDimension)
	}
	return cfg.Width, cfg.Height, nil
}

// fileCRC32 returns the IEEE CRC-32 of a file as 8 lower case hex digits.
func fileCRC32(p string) (string, error) {
	fh, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer fh.Close()
	h := crc32.NewIEEE()
	if _, err := io.Copy(h, io.LimitReader(fh, MaxImageSize+1)); err != nil {
		return "", err
	}
	return fmt.Sprintf("%08x", h.Sum32()), nil
}

// SupersededBoxArt returns the legacy boxart.png in the game directory dir
// when boxart.webp is also there, the only artwork Clean Library removes.
// ok is false when either file is missing.
func SupersededBoxArt(dir string) (path string, ok bool) {
	webp, err := os.Stat(filepath.Join(dir, "boxart.webp"))
	if err != nil || !webp.Mode().IsRegular() {
		return "", false
	}
	legacy := filepath.Join(dir, legacyBoxArt)
	info, err := os.Stat(legacy)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return legacy, true
}

// BoxArtPath returns the path of the front cover shown for a game in its
// game directory dir: the first front in LocalFiles order, so a front with
// no region, then USA, Japan, Europe and any other. A .webp file is used
// over the legacy boxart.png. When the game has no front, the boxart.webp
// path is returned.
func BoxArtPath(dir string) string {
	for _, f := range LocalFiles(dir) {
		if f.Type == "boxart" {
			return f.Path
		}
	}
	return filepath.Join(dir, "boxart.webp")
}
