package corpus

import "math/rand"

// Profile is a named, reproducible corpus. Plant writes it under root and
// returns the planted files. Each profile seeds its own RNG from a fixed seed,
// so the same profile yields byte-identical content on every run and machine —
// the property that lets a published benchmark number be independently verified.
type Profile struct {
	Name string
	Desc string
	// seed is the fixed RNG seed for this profile's random content.
	seed int64
	gen  func(root string, rng *rand.Rand) ([]File, error)
}

// Plant writes the profile under root with a freshly-seeded RNG.
func (p Profile) Plant(root string) ([]File, error) {
	return p.gen(root, rand.New(rand.NewSource(p.seed))) //nolint:gosec // deterministic corpus, not crypto
}

// Profiles returns the standard benchmark corpora, each reproducible.
//
//   - many-tiny:  request-count / small-object overhead (CargoShip's packing edge)
//   - few-large:  multipart uploads (large, incompressible)
//   - mixed:      both compressed (.tar.zst) and plain (.tar) chunks in one upload
//   - hostile:    awkward names/sizes/nesting, tiny edge cases (direct-upload path)
func Profiles() []Profile {
	return []Profile{
		{
			Name: "many-tiny", Desc: "5000 small random files across subdirs", seed: 0x746E79,
			gen: func(root string, rng *rand.Rand) ([]File, error) { return PlantManyFiles(root, rng, 5000) },
		},
		{
			Name: "few-large", Desc: "large incompressible files (multipart)", seed: 0x6C617267,
			gen: func(root string, rng *rand.Rand) ([]File, error) {
				return PlantSized(root, rng, []int{12 << 20, 8 << 20, 20 << 20, 6 << 20})
			},
		},
		{
			Name: "mixed", Desc: "compressible + already-compressed → both chunk kinds", seed: 0x6D697865,
			gen: func(root string, rng *rand.Rand) ([]File, error) {
				comp, err := PlantCompressible(root, []int{6 << 20, 6 << 20})
				if err != nil {
					return nil, err
				}
				blob, err := PlantAlreadyCompressed(root, rng, []int{6 << 20, 6 << 20})
				if err != nil {
					return nil, err
				}
				return append(comp, blob...), nil
			},
		},
		{
			Name: "hostile", Desc: "awkward names/sizes/nesting, tiny edge cases", seed: 0x686F7374,
			gen: func(root string, rng *rand.Rand) ([]File, error) { return PlantHostile(root, rng, 0) },
		},
	}
}

// ProfileByName returns the named profile and whether it exists.
func ProfileByName(name string) (Profile, bool) {
	for _, p := range Profiles() {
		if p.Name == name {
			return p, true
		}
	}
	return Profile{}, false
}
