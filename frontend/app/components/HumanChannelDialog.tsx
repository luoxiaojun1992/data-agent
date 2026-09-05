'use client';

import React, { useState } from 'react';
import {
  modalOverlayStyle,
  modalPanelStyle,
  modalInputStyle,
  modalLabelStyle,
  primaryButtonStyle,
  modalCancelBtnStyle,
} from './ui';

// HumanChannelEvent mirrors the backend confirm/ask event pushed on the
// human-channel SSE stream.
export type HumanChannelEvent = {
  type: 'confirm' | 'ask';
  request_id: string;
  hint?: string;
  question?: string;
  options?: string[];
};

// HumanChannelReply mirrors the reply payload posted back to the backend.
export type HumanChannelReply = { confirmed?: boolean; answer?: string };

/**
 * HumanChannelDialog renders the confirm/ask prompts for the human-in-the-loop
 * channel (SPEC-089). Styling reuses the SPEC-085 glass panel constants so it
 * matches every other modal in the app.
 */
export default function HumanChannelDialog({
  event,
  onReply,
}: {
  event: HumanChannelEvent;
  onReply: (reply: HumanChannelReply) => void;
}) {
  const [selected, setSelected] = useState<string>('');
  const [text, setText] = useState('');

  const isConfirm = event.type === 'confirm';

  const submitAsk = () => {
    // Prefer a picked option; fall back to free text (trimmed).
    const answer = selected !== '' ? selected : text.trim();
    onReply({ answer });
  };

  return (
    <div style={modalOverlayStyle} data-testid="human-channel-dialog">
      <div style={modalPanelStyle} onClick={(e) => e.stopPropagation()}>
        <h3 className="text-lg font-semibold text-[var(--text-primary)] mb-3">
          {isConfirm ? '操作确认' : '需要您的输入'}
        </h3>

        {isConfirm ? (
          <>
            <p
              className="text-sm text-[var(--text-secondary)] mb-5 break-words"
              data-testid="human-channel-hint"
            >
              {event.hint || '确认执行该操作？'}
            </p>
            <div className="flex justify-end gap-3">
              <button
                type="button"
                style={modalCancelBtnStyle}
                data-testid="human-channel-deny"
                onClick={() => onReply({ confirmed: false })}
              >
                拒绝
              </button>
              <button
                type="button"
                style={primaryButtonStyle}
                data-testid="human-channel-confirm"
                onClick={() => onReply({ confirmed: true })}
              >
                确认
              </button>
            </div>
          </>
        ) : (
          <>
            <p
              className="text-sm text-[var(--text-primary)] mb-4 break-words"
              data-testid="human-channel-question"
            >
              {event.question}
            </p>

            {(event.options?.length ?? 0) > 0 && (
              <div className="flex flex-col gap-2 mb-4">
                {event.options!.map((opt, i) => (
                  <button
                    key={i}
                    type="button"
                    data-testid={`human-channel-option-${i}`}
                    onClick={() => setSelected(opt)}
                    className={`text-left px-3 py-2 rounded-lg text-sm border transition-colors ${
                      selected === opt
                        ? 'bg-[var(--accent)]/20 border-[var(--accent)] text-[var(--text-primary)]'
                        : 'bg-white/5 border-white/10 text-[var(--text-secondary)] hover:bg-white/10'
                    }`}
                  >
                    {opt}
                  </button>
                ))}
              </div>
            )}

            <div className="mb-4">
              <label style={modalLabelStyle} htmlFor="human-channel-input">
                或直接输入
              </label>
              <input
                id="human-channel-input"
                style={modalInputStyle}
                data-testid="human-channel-input"
                value={text}
                placeholder="输入你的回答…"
                onChange={(e) => setText(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault();
                    submitAsk();
                  }
                }}
              />
            </div>

            <div className="flex justify-end gap-3">
              <button
                type="button"
                style={primaryButtonStyle}
                data-testid="human-channel-submit"
                onClick={submitAsk}
              >
                提交
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
