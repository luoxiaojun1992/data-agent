'use client';

// RedactOverlay — 脱敏中弹窗动画（SPEC-093 §5.3.1）。
// 全屏半透明遮罩 + 中央安全盾牌 + 环绕光环 + 「脱敏中…」。
// 纯 CSS 动画（无额外依赖）；不可手动关闭，由调用方控制渲染。

export default function RedactOverlay() {
  return (
    <div
      className="fixed inset-0 z-[9999] flex items-center justify-center"
      style={{ background: 'rgba(0,0,0,0.5)' }}
      data-testid="chat-redact-overlay"
      onClick={(e) => e.stopPropagation()}
    >
      <style>{`
        @keyframes redact-ring-spin {
          from { transform: rotate(0deg); }
          to { transform: rotate(360deg); }
        }
        @keyframes redact-shield-pulse {
          0%, 100% { transform: scale(1); }
          50% { transform: scale(1.06); }
        }
        @keyframes redact-ring-breathe {
          0%, 100% { opacity: 0.9; }
          50% { opacity: 0.55; }
        }
      `}</style>

      <div className="flex flex-col items-center gap-5">
        {/* 盾牌 + 旋转光环 */}
        <div className="relative" style={{ width: 96, height: 96 }}>
          <div
            className="absolute inset-0 rounded-full"
            style={{
              border: '2px dashed var(--accent, #5c7cfa)',
              animation: 'redact-ring-spin 2.4s linear infinite, redact-ring-breathe 2.4s ease-in-out infinite',
            }}
          />
          <div
            className="absolute inset-3 rounded-full"
            style={{
              border: '1px solid rgba(92,124,250,0.35)',
              animation: 'redact-ring-spin 1.6s linear infinite reverse',
            }}
          />
          {/* 盾牌 SVG */}
          <div
            className="absolute inset-0 flex items-center justify-center"
            style={{ animation: 'redact-shield-pulse 1.4s ease-in-out infinite' }}
            data-testid="chat-redact-shield"
          >
            <svg width="48" height="48" viewBox="0 0 24 24" fill="none">
              <path
                d="M12 2.5 20 5.5v5.4c0 5-3.4 8.6-8 10.6-4.6-2-8-5.6-8-10.6V5.5L12 2.5z"
                fill="var(--accent, #5c7cfa)"
                fillOpacity="0.18"
                stroke="var(--accent, #5c7cfa)"
                strokeWidth="1.4"
              />
              <path
                d="M8.6 12.1l2.2 2.2 4.6-4.9"
                stroke="var(--accent, #5c7cfa)"
                strokeWidth="1.7"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </svg>
          </div>
        </div>

        <p className="text-sm font-medium" style={{ color: 'var(--text-primary, #fff)' }}>
          脱敏中…
        </p>
      </div>
    </div>
  );
}
