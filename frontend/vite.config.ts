import {defineConfig} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'
import {resolve} from 'path'

// Two entry points, not one: index.html is the app and logviewer.html is the detachable debug
// log window, which the backend opens as its own subprocess with its own document. Dropping
// either from this map silently stops building it - the app keeps working and the log window
// serves a 404, which is a long way from its cause.
//
// `rollupOptions` still, on vite 8. Vite 8 bundles with rolldown rather than rollup and its
// build log now suggests `build.rolldownOptions`, but the old key remains supported and is what
// the vite docs still use for `input`. Left alone deliberately: renaming it buys nothing and
// this is the one config that decides whether the log viewer exists.
export default defineConfig({
  plugins: [svelte()],
  build: {
    rollupOptions: {
      input: {
        main: resolve(__dirname, 'index.html'),
        logviewer: resolve(__dirname, 'logviewer.html'),
      },
    },
  },
})
