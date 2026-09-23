// SPEC-103 语言偏好常量：cookie（SSR 可读）+ localStorage 镜像。
// cookie 名沿用 next-intl 约定的 NEXT_LOCALE。
export const LOCALE_COOKIE = 'NEXT_LOCALE';
export const LOCALE_STORAGE_KEY = 'data-agent-locale';

export type Locale = 'zh' | 'en';
export const DEFAULT_LOCALE: Locale = 'zh';
