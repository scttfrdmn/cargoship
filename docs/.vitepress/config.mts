import { defineConfig } from 'vitepress'
import fs from 'node:fs'
import path from 'node:path'

const SITE_URL = 'https://cargoship.app'

// Versioned docs (task #37). The site deploys two trees from one config:
//   /      → "latest": docs from the newest release tag that contains the site
//            (falls back to main until a release ships the site)
//   /dev/  → "dev": docs built from main (unreleased)
// The deploy workflow sets DOCS_BASE ('/' or '/dev/') and DOCS_VERSION_LABEL
// per build. The version switcher (below) uses absolute SITE_URL links so it
// navigates between trees regardless of the active base.
const DOCS_BASE = process.env.DOCS_BASE || '/'
const DOCS_VERSION_LABEL = process.env.DOCS_VERSION_LABEL || 'latest'
const IS_DEV_TREE = DOCS_BASE === '/dev/'

// One global sidebar (keyed on '/') applied to every page, so the whole site
// reads as a single progressive-disclosure path: Introduction → Get Started →
// Tutorials → Core Workflows → Cost & Budget → Features → DVC → Configuration →
// Reference → Format Spec → Distributed → Project. This is deliberate — a single
// mental model rather than a different sidebar per section. The `llms.txt`
// generator (buildEnd, below) walks this exact structure so the manifest can't
// drift from the nav.
const sidebar = {
  '/': [
    {
      text: 'Introduction',
      collapsed: false,
      items: [
        { text: 'What is CargoShip?', link: '/' },
        { text: 'How it works (the pipeline)', link: '/intro/how-it-works' },
        { text: 'Concepts & terminology', link: '/intro/concepts' },
        { text: 'Costs & safety guarantees', link: '/intro/costs-and-safety' },
      ],
    },
    {
      text: 'Get Started',
      collapsed: false,
      items: [
        { text: 'Quick Start', link: '/start/quickstart' },
        { text: 'Install', link: '/start/install' },
        { text: 'AWS setup & credentials', link: '/start/aws-setup' },
        { text: 'Your first upload', link: '/start/first-upload' },
        { text: 'Verify & restore it', link: '/start/verify-and-restore' },
        { text: 'Clean up', link: '/start/cleanup' },
      ],
    },
    {
      text: 'Tutorials (by use-case)',
      collapsed: false,
      items: [
        { text: 'Overview', link: '/tutorials/' },
        { text: 'Genomics / sequencing data', link: '/tutorials/genomics' },
        { text: 'Imaging / microscopy data', link: '/tutorials/imaging' },
        { text: 'ML datasets with DVC', link: '/tutorials/ml-dvc' },
        { text: 'Lab data manager', link: '/tutorials/lab-manager' },
        { text: 'Principal investigator', link: '/tutorials/principal-investigator' },
        { text: 'Migrating from rclone / aws cli', link: '/tutorials/migrating' },
      ],
    },
    {
      text: 'Core Workflows',
      collapsed: false,
      items: [
        { text: 'Uploading data', link: '/guides/uploading' },
        { text: 'upload vs. create upload', link: '/guides/upload-vs-create-upload' },
        { text: 'Incremental sync', link: '/guides/sync' },
        { text: 'Listing & inspecting uploads', link: '/guides/inspecting' },
        { text: 'Downloading & extracting', link: '/guides/downloading' },
        { text: 'Verifying integrity', link: '/guides/verifying' },
        { text: 'Restoring files (incl. Glacier)', link: '/guides/restoring' },
        { text: 'Browsing archives (TUI/shell)', link: '/guides/browsing' },
        { text: 'Resuming interrupted uploads', link: '/guides/resuming' },
      ],
    },
    {
      text: 'Cost & Budget',
      collapsed: false,
      items: [
        { text: 'Estimating costs', link: '/guides/cost/estimate' },
        { text: 'Analyzing existing S3 spend', link: '/guides/cost/analyze' },
        { text: 'Cost management & reporting', link: '/guides/cost/management' },
        { text: 'Budgets & volume quotas', link: '/guides/cost/budgets' },
        { text: 'Alerts & notifications', link: '/guides/cost/alerts' },
        { text: 'Lifecycle & storage classes', link: '/guides/cost/lifecycle' },
      ],
    },
    {
      text: 'Features & Optimization',
      collapsed: true,
      items: [
        { text: 'Compression & content-aware', link: '/guides/features/compression' },
        { text: 'Magika AI file detection', link: '/guides/features/magika' },
        { text: 'Multi-prefix sharding', link: '/guides/features/sharding' },
        { text: 'Multi-region (LB & failover)', link: '/guides/features/multi-region' },
        { text: 'Encryption (KMS & GPG)', link: '/guides/features/encryption' },
        { text: 'Tier-aware storage', link: '/guides/features/tiering' },
        { text: 'Performance tuning', link: '/guides/features/optimization' },
        { text: 'Observability & tracing', link: '/guides/features/observability' },
        { text: 'Benchmarking', link: '/guides/features/benchmarking' },
      ],
    },
    {
      text: 'DVC Integration',
      collapsed: true,
      items: [
        { text: 'Overview', link: '/guides/dvc/' },
        { text: 'Go `dvc` command', link: '/guides/dvc/command' },
        { text: 'Python dvc-cargoship plugin', link: '/guides/dvc/plugin' },
      ],
    },
    {
      text: 'Configuration',
      collapsed: true,
      items: [
        { text: 'Config files & precedence', link: '/guides/config/files' },
        { text: 'Interactive setup wizard', link: '/guides/config/setup' },
        { text: 'Execution contexts', link: '/guides/config/contexts' },
        { text: 'Shell completion', link: '/guides/config/completion' },
      ],
    },
    {
      text: 'Reference',
      collapsed: true,
      items: [
        { text: 'Command reference overview', link: '/reference/' },
        { text: 'Uploading & sync commands', link: '/reference/commands/upload' },
        { text: 'Inspection & retrieval', link: '/reference/commands/inspect' },
        { text: 'Cost, budget & alerts', link: '/reference/commands/cost' },
        { text: 'DVC commands', link: '/reference/commands/dvc' },
        { text: 'Configuration & context', link: '/reference/commands/config' },
        { text: 'Destructive operations', link: '/reference/commands/destructive' },
        { text: 'Diagnostics & utilities', link: '/reference/commands/diagnostics' },
        { text: 'Global flags', link: '/reference/commands/global-flags' },
        { text: 'Environment variables', link: '/reference/environment-variables' },
        { text: 'Configuration schema', link: '/reference/configuration' },
        { text: 'CargoShip vs. other tools', link: '/reference/comparison' },
        { text: 'Benchmarks & methodology', link: '/reference/benchmarks' },
        { text: 'Recovery & operations runbook', link: '/reference/recovery' },
        { text: 'Troubleshooting', link: '/reference/troubleshooting' },
        { text: 'FAQ', link: '/reference/faq' },
        { text: 'Glossary', link: '/reference/glossary' },
        { text: 'Cheat sheet', link: '/reference/cheatsheet' },
      ],
    },
    {
      text: 'Archive & Manifest Format Spec',
      collapsed: true,
      items: [
        { text: 'Format spec overview', link: '/reference/format/' },
        { text: 'Archive layout & shard keys', link: '/reference/format/archive-layout' },
        { text: 'Compression levels', link: '/reference/format/compression' },
        { text: 'Manifest schema (v2.0)', link: '/reference/format/manifest' },
        { text: 'Encryption metadata', link: '/reference/format/encryption' },
        { text: 'Split-file PAX records', link: '/reference/format/split-files' },
        { text: 'Reading archives (Go library)', link: '/reference/format/library-api' },
      ],
    },
    {
      text: 'Distributed / Enterprise',
      collapsed: true,
      items: [
        { text: 'Overview', link: '/enterprise/' },
        { text: 'Launch agents', link: '/enterprise/launch-agent' },
        { text: 'Controller', link: '/enterprise/controller' },
        { text: 'ghost-ship', link: '/enterprise/ghost-ship' },
        { text: 'Web UI', link: '/enterprise/webui' },
        { text: 'Deployment guide', link: '/enterprise/deployment' },
        { text: 'QNAP / NAS deployment', link: '/enterprise/qnap' },
      ],
    },
    {
      text: 'Project',
      collapsed: true,
      items: [
        { text: 'Architecture', link: '/project/architecture' },
        { text: 'Project maturity & compatibility', link: '/project/maturity' },
        { text: 'Security model', link: '/project/security' },
        { text: 'API stability & versioning', link: '/project/versioning' },
        { text: 'Attribution & license', link: '/project/attribution' },
      ],
    },
  ],
}

// Build an llms.txt manifest (https://llmstxt.org) from the sidebar so AI
// assistants get a curated, always-current map of the docs. Written to the build
// output dir at build time; regenerated on every build, so it can't go stale.
function writeLlmsTxt(outDir: string) {
  const lines: string[] = []
  lines.push('# CargoShip documentation')
  lines.push('')
  lines.push('> High-performance S3 data archiving. Stream large datasets straight to S3 — sharded, compressed, verifiable, cost-aware — in an open, portable archive format (tar.zst objects + a documented JSON manifest).')
  lines.push('')
  const link = (item: any) =>
    item.link && item.link.startsWith('/') && !item.link.includes('#')
      ? `- [${item.text}](${SITE_URL}${item.link})`
      : null
  for (const group of sidebar['/']) {
    lines.push(`## ${group.text}`)
    lines.push('')
    for (const item of group.items ?? []) {
      const l = link(item)
      if (l) lines.push(l)
    }
    lines.push('')
  }
  fs.writeFileSync(path.join(outDir, 'llms.txt'), lines.join('\n'))
}

export default defineConfig({
  title: 'CargoShip',
  description: 'High-performance S3 data archiving.',
  lang: 'en-US',
  cleanUrls: true,
  lastUpdated: true,

  // Set per deploy: '/' for the latest tree, '/dev/' for the unreleased tree.
  base: DOCS_BASE,

  // Emit sitemap.xml for search + AI indexers (VitePress only generates it when
  // a hostname is set). Only the latest (root) tree is indexed; the dev tree is
  // excluded from the sitemap and marked noindex (see head, below) so search
  // engines don't surface unreleased docs as canonical.
  sitemap: IS_DEV_TREE ? undefined : {
    hostname: SITE_URL,
  },

  // The vendored cobra fragments are @included by the reference command pages —
  // don't build them as standalone routes.
  srcExclude: ['gen/**'],

  head: [
    ['link', { rel: 'preconnect', href: 'https://fonts.googleapis.com' }],
    ['link', { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: '' }],
    ['link', { href: 'https://fonts.googleapis.com/css2?family=Atkinson+Hyperlegible:ital,wght@0,400;0,700;1,400;1,700&family=Atkinson+Hyperlegible+Mono:ital,wght@0,400;0,700;1,400;1,700&display=swap', rel: 'stylesheet' }],
    ['link', { rel: 'icon', type: 'image/png', href: '/logo-512.png' }],
    // Default OpenGraph/Twitter card metadata (per-page title/description still
    // override via frontmatter). og-cover.png is a user-supplied asset slot.
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:site_name', content: 'CargoShip' }],
    ['meta', { property: 'og:title', content: 'CargoShip — high-performance S3 data archiving' }],
    ['meta', { property: 'og:description', content: 'Stream large datasets straight to S3 — sharded, compressed, verifiable, cost-aware, in an open portable format.' }],
    ['meta', { property: 'og:url', content: SITE_URL + '/' }],
    ['meta', { property: 'og:image', content: SITE_URL + '/og-cover.png' }],
    ['meta', { name: 'twitter:card', content: 'summary_large_image' }],
    ['meta', { name: 'twitter:image', content: SITE_URL + '/og-cover.png' }],
    // The dev tree (built from main) must not be indexed as canonical — the
    // latest release docs at the root are authoritative.
    ...(IS_DEV_TREE ? [['meta', { name: 'robots', content: 'noindex' }] as const] : []),
  ],

  themeConfig: {
    siteTitle: 'CargoShip',
    logo: '/logo-512.png',

    nav: [
      { text: 'Get Started', link: '/start/quickstart' },
      { text: 'Tutorials', link: '/tutorials/' },
      { text: 'Guides', link: '/guides/uploading' },
      { text: 'Reference', link: '/reference/' },
      { text: 'Format Spec', link: '/reference/format/' },
      // Version switcher. Absolute links so it crosses between the latest (root)
      // and dev trees regardless of which base this build was rendered with.
      {
        text: DOCS_VERSION_LABEL,
        items: [
          { text: 'latest (released)', link: SITE_URL + '/' },
          { text: 'dev (main)', link: SITE_URL + '/dev/' },
        ],
      },
      {
        text: 'Get help',
        items: [
          { text: '🐛 Report a problem', link: 'https://github.com/scttfrdmn/cargoship/issues/new/choose' },
          { text: '💬 Discussions', link: 'https://github.com/scttfrdmn/cargoship/discussions' },
          { text: '🔒 Report a vulnerability (private)', link: 'https://github.com/scttfrdmn/cargoship/security/advisories/new' },
        ],
      },
      { text: 'GitHub', link: 'https://github.com/scttfrdmn/cargoship', target: '_blank' },
    ],

    sidebar,

    socialLinks: [
      { icon: 'github', link: 'https://github.com/scttfrdmn/cargoship' },
    ],

    editLink: {
      pattern: 'https://github.com/scttfrdmn/cargoship/edit/main/docs/:path',
      text: 'Edit this page on GitHub',
    },

    search: {
      provider: 'local',
    },

    footer: {
      message: 'Released under the <a href="https://github.com/scttfrdmn/cargoship/blob/main/LICENSE">Apache 2.0 License</a>. Built on the foundation of <a href="https://gitlab.oit.duke.edu/devil-ops/suitcasectl">SuitcaseCTL</a> by Duke University.',
      copyright: '© 2025–2026 Scott Friedman',
    },
  },

  // Generate llms.txt (AI-assistant manifest) into the build output on every
  // build, derived from the sidebar above so it can't drift from the nav.
  buildEnd(siteConfig) {
    writeLlmsTxt(siteConfig.outDir)
  },
})
