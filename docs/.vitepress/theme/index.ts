import DefaultTheme from 'vitepress/theme'
import { h } from 'vue'
import './custom.css'

// The version comes from internal/version/version.txt — the single canonical
// source (PR #260), the same file the release process bumps and
// scripts/ci/check-doc-versions.sh enforces. It used to be a hardcoded constant
// here with an "update this when cutting a release" comment, which went stale by
// four releases (it still advertised v0.13.2 on the day v0.17.0 shipped)
// because nothing checked it.
// Both values are injected at build time via `define` in config.mts, which
// reads them on the Node side. This theme file is compiled into the browser
// bundle, so it can neither read the filesystem nor see process.env.
const LATEST_RELEASE = __CARGOSHIP_VERSION__

// The banner only belongs on the dev tree. The root tree IS the latest release,
// so telling its readers "these docs track main" was wrong as well as stale.
const IS_DEV_TREE = __CARGOSHIP_IS_DEV_TREE__

export default {
  extends: DefaultTheme,
  Layout() {
    return h(DefaultTheme.Layout, null, {
      'layout-top': () =>
        IS_DEV_TREE
          ? h(
              'div',
              { class: 'cs-dev-banner' },
              [
                'These docs track the development branch (',
                h('code', null, 'main'),
                `). Latest release: ${LATEST_RELEASE}.`,
              ],
            )
          : null,
    })
  },
}
