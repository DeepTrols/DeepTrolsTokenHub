import tailwindcss from '@tailwindcss/vite'

export default defineNuxtConfig({
  compatibilityDate: '2026-08-25',
  devtools: { enabled: false },
  app: {
    baseURL: '/ai/',
    head: {
      htmlAttrs: {
        lang: 'zh-CN',
      },
      title: '智曜TokenHub',
      meta: [
        { name: 'theme-color', content: '#ffffff' },
        {
          name: 'description',
          content:
            '统一的 AI 模型聚合与分发网关，为个人与企业提供集中式模型管理、OpenAI 兼容接口和透明模型价格。',
        },
      ],
      link: [{ rel: 'icon', href: '/ai/logo.png' }],
    },
  },
  css: ['~/assets/css/tailwind.css', '~/assets/scss/main.scss'],
  components: [
    {
      path: '~/components',
      pathPrefix: false,
    },
  ],
  vite: {
    plugins: [tailwindcss()],
  },
  nitro: {
    prerender: {
      crawlLinks: false,
      routes: ['/', '/pricing', '/console', '/login', '/register', '/reset-password'],
    },
  },
  typescript: {
    strict: true,
    typeCheck: true,
  },
})
