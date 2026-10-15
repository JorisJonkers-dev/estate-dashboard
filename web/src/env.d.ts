/// <reference types="vite/client" />

// Types .vue imports for ESLint. vue-tsc has the real types.
declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent
  export default component
}
