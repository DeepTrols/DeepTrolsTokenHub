<template>
  <form class="flex flex-col justify-center px-10 py-11 max-[640px]:px-6 max-[640px]:py-9">
    <h2 class="m-0 mb-[34px] text-[22px] font-black leading-[normal] text-white">创建账户</h2>

    <label class="mb-7 flex flex-col gap-2 text-[15px] font-bold leading-[normal] text-auth-label">
      <span>用户名</span>
      <span class="flex h-[41px] items-center gap-2 rounded-[9px] bg-auth-input-bg px-3 text-auth-input-icon">
        <span>◎</span>
        <input
          type="text"
          class="min-w-0 flex-1 border-0 bg-transparent text-white outline-none placeholder:text-auth-placeholder"
          placeholder="请输入用户名"
          autocomplete="username"
        />
      </span>
    </label>

    <label class="mb-7 flex flex-col gap-2 text-[15px] font-bold leading-[normal] text-auth-label">
      <span>邮箱地址</span>
      <span class="flex h-[41px] items-center gap-2 rounded-[9px] bg-auth-input-bg px-3 text-auth-input-icon">
        <span>✉</span>
        <input
          type="email"
          class="min-w-0 flex-1 border-0 bg-transparent text-white outline-none placeholder:text-auth-placeholder"
          placeholder="请输入邮箱地址"
          autocomplete="email"
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
          placeholder="请设置登录密码"
          autocomplete="new-password"
        />
      </span>
    </label>

    <button
      class="mb-[22px] mt-2 h-[54px] cursor-pointer rounded-[10px] border-0 bg-gradient-to-br from-brand to-auth-submit-end text-base font-black leading-[normal] text-white shadow-[0_14px_30px_rgba(255,133,0,0.32)]"
      type="button"
    >
      注 册
    </button>

    <p class="m-0 mt-[30px] text-center text-sm leading-[normal] text-auth-muted">
      已有账户?
      <NuxtLink to="/login" class="font-extrabold text-brand">立即登录</NuxtLink>
    </p>
  </form>
</template>
