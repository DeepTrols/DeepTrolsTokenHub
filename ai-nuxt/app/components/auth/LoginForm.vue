<template>
  <form class="flex flex-col justify-center px-10 py-11 max-[640px]:px-6 max-[640px]:py-9">
    <h2 class="m-0 mb-[34px] text-[22px] font-black leading-[normal] text-white">账号登录</h2>

    <label class="mb-7 flex flex-col gap-2 text-[15px] font-bold leading-[normal] text-auth-label">
      <span>用户名或邮箱</span>
      <span class="flex h-[41px] items-center gap-2 rounded-[9px] bg-auth-input-bg px-3 text-auth-input-icon">
        <span>✉</span>
        <input
          type="text"
          class="min-w-0 flex-1 border-0 bg-transparent text-white outline-none placeholder:text-auth-placeholder"
          placeholder="请输入用户名或邮箱地址"
          autocomplete="username"
        />
      </span>
    </label>

    <label class="mb-7 flex flex-col gap-2 text-[15px] font-bold leading-[normal] text-auth-label">
      <span>密码</span>
      <span class="flex h-[41px] items-center gap-2 rounded-[9px] bg-auth-input-bg px-3 text-auth-input-icon">
        <span>■</span>
        <input
          type="password"
          class="min-w-0 flex-1 border-0 bg-transparent text-white outline-none placeholder:text-auth-placeholder"
          placeholder="请输入您的密码"
          autocomplete="current-password"
        />
        <button type="button" class="cursor-pointer border-0 bg-transparent text-auth-input-action" aria-label="显示密码">◌</button>
      </span>
    </label>

    <button
      class="mb-[22px] mt-2 h-[54px] cursor-pointer rounded-[10px] border-0 bg-gradient-to-br from-brand to-auth-submit-end text-base font-black leading-[normal] text-white shadow-[0_14px_30px_rgba(255,133,0,0.32)]"
      type="button"
    >
      登 录
    </button>

    <NuxtLink to="/reset-password" class="text-center text-sm leading-[normal] text-auth-muted">忘记密码?</NuxtLink>

    <p class="m-0 mt-[30px] text-center text-sm leading-[normal] text-auth-muted">
      没有账户?
      <NuxtLink to="/register" class="font-extrabold text-brand">立即注册</NuxtLink>
    </p>
  </form>
</template>
