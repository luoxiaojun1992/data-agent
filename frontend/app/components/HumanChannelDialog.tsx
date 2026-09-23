'use client';

import React, { useState } from 'react';
import { useTranslations } from 'next-intl';
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
  const t = useTranslations('humanChannel');

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
          {isConfirm ? t('confirmTitle') : t('askTitle')}
        </h3>

        {isConfirm ? (
          <>
            <p
              className="text-sm text-[var(--text-secondary)] mb-5 break-words"
              data-testid="human-channel-hint"
            >
              {event.hint || t('confirmHint')}
            </p>
            <div className="flex justify-end gap-3">
              <button
                type="button"
                style={modalCancelBtnStyle}
                data-testid="human-channel-deny"
                onClick={() => onReply({ confirmed: false })}
              >
                {t('deny')}
              </button>
              <button
                type="button"
                style={primaryButtonStyle}
                data-testid="human-channel-confirm"
                onClick={() => onReply({ confirmed: true })}
              >
                {t('confirm')}
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
                        : 'bg-[var(--surface-5)] border-[var(--surface-10)] text-[var(--text-secondary)] hover:bg-[var(--surface-10)]'
                    }`}
                  >
                    {opt}
                  </button>
                ))}
              </div>
            )}

            <div className="mb-4">
              <label style={modalLabelStyle} htmlFor="human-channel-input">
                {t('orInput')}
              </label>
              <input
                id="human-channel-input"
                style={modalInputStyle}
                data-testid="human-channel-input"
                value={text}
                placeholder={t('answerPlaceholder')}
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
                {t('submit')}
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
