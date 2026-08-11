import "@testing-library/jest-dom";

// jsdom 未实现 ResizeObserver，recharts 的 ResponsiveContainer 在挂载时依赖它，
// 缺失会导致整棵渲染树崩溃（ReferenceError）。
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver;
}

// jsdom 未实现 matchMedia（recharts 与响应式组件会读取）。
if (typeof window.matchMedia !== "function") {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}

// jsdom 未实现 IntersectionObserver（Radix 弹层 / 懒加载会用）。
class IntersectionObserverStub {
  readonly root: Element | null = null;
  readonly rootMargin: string = "";
  readonly thresholds: ReadonlyArray<number> = [];
  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords(): IntersectionObserverEntry[] {
    return [];
  }
}
if (typeof globalThis.IntersectionObserver === "undefined") {
  globalThis.IntersectionObserver = IntersectionObserverStub as unknown as typeof IntersectionObserver;
}

// jsdom 未实现 window.scrollTo / Element.prototype.scrollIntoView。
if (typeof window.scrollTo !== "function") {
  window.scrollTo = (() => {}) as typeof window.scrollTo;
}
if (typeof Element !== "undefined" && typeof Element.prototype.scrollIntoView !== "function") {
  Element.prototype.scrollIntoView = (() => {}) as typeof Element.prototype.scrollIntoView;
}

// jsdom 未实现 Pointer 指针捕获 API，Radix Select 的 pointer 处理会调用
// target.hasPointerCapture / setPointerCapture / releasePointerCapture，缺失会抛 TypeError。
if (typeof Element !== "undefined") {
  if (typeof Element.prototype.hasPointerCapture !== "function") {
    Element.prototype.hasPointerCapture = (() => false) as typeof Element.prototype.hasPointerCapture;
  }
  if (typeof Element.prototype.setPointerCapture !== "function") {
    Element.prototype.setPointerCapture = (() => {}) as typeof Element.prototype.setPointerCapture;
  }
  if (typeof Element.prototype.releasePointerCapture !== "function") {
    Element.prototype.releasePointerCapture = (() => {}) as typeof Element.prototype.releasePointerCapture;
  }
}

// jsdom 的 window.confirm 未实现（调用返回 undefined，等同于"取消"）。
// 测试中默认"确认"，让 confirm 守卫的操作能够继续；需要取消的用例会自行 mockReturnValue(false)。
window.confirm = (() => true) as typeof window.confirm;
