# 智曜算力超市 Nuxt 工程

Nuxt 4 source for the `/ai` surface. The visible UI should stay aligned with the original design while being maintained as Vue components and Nuxt routes.

## Structure

```text
ai-nuxt/
├── app/
│   ├── assets/
│   │   ├── css/tailwind.css
│   │   └── scss/main.scss
│   ├── components/
│   │   ├── app/
│   │   ├── auth/
│   │   ├── home/
│   │   ├── pricing/
│   │   └── ui/
│   ├── data/
│   ├── pages/
│   │   ├── index.vue
│   │   ├── login.vue
│   │   ├── pricing.vue
│   │   ├── console.vue
│   │   ├── register.vue
│   │   └── reset-password.vue
│   ├── types/
│   └── app.vue
├── public/
│   └── logo.png
├── server/api/
├── nuxt.config.ts
├── package.json
├── pnpm-lock.yaml
└── tsconfig.json
```

## Scripts

```sh
pnpm install
pnpm dev
pnpm typecheck
pnpm build
```

`pnpm dev` starts Nuxt at:

```text
http://127.0.0.1:4173/ai/
```
