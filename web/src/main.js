import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'
import { initTracking } from './lib/tracking.js'

const app = mount(App, { target: document.getElementById('app') })

// 统计追踪（Umami 页面/事件 + 底部自定义 JS，互不影响）；配置来自后台 /admin
initTracking()

export default app
