import { useI18n } from '../i18n';
import { ACTIONS, EMERGENCY_NUMBERS } from '../content/safety';
import { IconPhone } from './icons';

export function Actions() {
  const { t, lang } = useI18n();
  return (
    <section class="todo" aria-labelledby="todo-h">
      <h2 id="todo-h" class="section-title">
        {t.whatToDo}
      </h2>
      <ol class="action-list">
        {ACTIONS[lang].map((a) => (
          <li key={a}>{a}</li>
        ))}
      </ol>
    </section>
  );
}

export function EmergencyNumbers({ invert = false }: { invert?: boolean }) {
  const { t, lang } = useI18n();
  return (
    <section aria-label={t.emergency} class="stack-sm">
      {!invert && <h2 class="section-label">{t.emergency}</h2>}
      <div class="call-row">
        {EMERGENCY_NUMBERS.map((n) => (
          <a key={n.number} class={invert ? 'call call-invert' : 'call'} href={`tel:${n.number}`}>
            <span class="call-icon" aria-hidden="true">
              <IconPhone size={20} />
            </span>
            <span class="call-num">{n.number}</span>
            <span class="call-label">{n.label[lang]}</span>
          </a>
        ))}
      </div>
    </section>
  );
}
