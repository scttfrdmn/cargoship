# Static assets & branding

> Lives in `.vitepress/` rather than `public/` on purpose: everything in `public/`
> is copied verbatim to the site root, so this note would be downloadable at
> `/README.md`. `.vitepress/` is not copied.

Files here are served from the site root (e.g. `logo-512.png` → `/logo-512.png`).

| File | Used for | Referenced from |
|------|----------|-----------------|
| `hero.png` | Landing-page hero image (the ship-on-a-cloud emblem) | `index.md` → `hero.image.src` |
| `og-cover.png` | Social / Open Graph card (1200×630 banner) | `og:image` / `twitter:image` in `.vitepress/config.mts` |
| `logo-512.png` | Nav-bar logo and browser favicon | `themeConfig.logo` and the `icon` link in `.vitepress/config.mts` |
| `CNAME` | Custom domain (`cargoship.app`) for GitHub Pages | Do not remove — VitePress wipes the build output dir, so the domain file must live here to survive deploys. |

To rebrand, replace the PNGs in place (keep the same filenames) or update the
references in `index.md` / `.vitepress/config.mts`.
