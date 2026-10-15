import '@fontsource/barlow-semi-condensed/latin-400.css'
import '@fontsource/barlow-semi-condensed/latin-500.css'
import '@fontsource/barlow-semi-condensed/latin-600.css'
import '@fontsource/ibm-plex-mono/latin-400.css'
import './style.css'
import { QueryCache, QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { createApp } from 'vue'
import App from './app/App.vue'
import { backToSignIn } from './app/expired'
import { createAppRouter } from './app/router'
import { configureApi } from './infrastructure/http'

configureApi()
const router = createAppRouter()
const queryClient = new QueryClient({ queryCache: new QueryCache({ onError: backToSignIn(router) }) })
createApp(App).use(router).use(VueQueryPlugin, { queryClient }).mount('#app')
