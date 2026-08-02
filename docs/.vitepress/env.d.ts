// Ambient declarations for the build-time constants injected by Vite `define`
// in config.mts. The theme runs in the browser bundle, so these are the only
// way it can see Node-side values (the canonical version, the active base).
declare const __CARGOSHIP_VERSION__: string
declare const __CARGOSHIP_IS_DEV_TREE__: boolean
