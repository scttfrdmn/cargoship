import DefaultTheme from 'vitepress/theme'
import { h } from 'vue'
import './custom.css'

// Site-wide banner noting that the docs track `main` (the development branch),
// not necessarily the latest tagged release. Update LATEST_RELEASE when cutting a
// release. Rendered via the built-in `layout-top` slot so it appears on every
// page, including the home layout.
const LATEST_RELEASE = 'v0.13.2'

export default {
  extends: DefaultTheme,
  Layout() {
    return h(DefaultTheme.Layout, null, {
      'layout-top': () =>
        h(
          'div',
          { class: 'cs-dev-banner' },
          [
            'These docs track the development branch (',
            h('code', null, 'main'),
            `). Latest release: ${LATEST_RELEASE}.`,
          ],
        ),
    })
  },
}
