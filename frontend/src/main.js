import { createApp } from 'vue'
import App from './App.vue'
import './shared/style.css'
// WebKit drags of images and links carry non-file URLs, which crash the Wails file-drop
// handler when released over the window. Selected text still drags as a Text node.
addEventListener('dragstart', (event) => event.target instanceof Element && event.preventDefault())
createApp(App).mount('#app')
