/* ─── notify.js ─── Shared toast + confirm system for AEFCUP ─── */

(function () {
  /* ── Inject styles once ── */
  if (!document.getElementById('notify-styles')) {
    const style = document.createElement('style');
    style.id = 'notify-styles';
    style.textContent = `
      /* Toast container */
      #notify-toast-container {
        position: fixed;
        bottom: 24px;
        right: 24px;
        z-index: 99999;
        display: flex;
        flex-direction: column;
        gap: 10px;
        pointer-events: none;
      }

      .notify-toast {
        display: flex;
        align-items: center;
        gap: 12px;
        padding: 14px 18px;
        border-radius: 12px;
        background: white;
        box-shadow: 0 8px 24px rgba(0,0,0,0.14);
        border-left: 5px solid #1a73e8;
        font-family: 'Inter', system-ui, sans-serif;
        font-size: 14px;
        font-weight: 500;
        color: #202124;
        min-width: 260px;
        max-width: 380px;
        pointer-events: all;
        animation: notify-slide-in 0.3s cubic-bezier(0.34,1.56,0.64,1) forwards;
      }

      .notify-toast.notify-success { border-color: #188038; }
      .notify-toast.notify-error   { border-color: #d93025; }
      .notify-toast.notify-warning { border-color: #f29900; }
      .notify-toast.notify-info    { border-color: #1a73e8; }

      .notify-toast .notify-icon { font-size: 18px; flex-shrink: 0; }
      .notify-toast .notify-msg  { flex: 1; line-height: 1.4; }

      .notify-toast.notify-hiding {
        animation: notify-slide-out 0.25s ease forwards;
      }

      @keyframes notify-slide-in {
        from { opacity: 0; transform: translateX(30px); }
        to   { opacity: 1; transform: translateX(0); }
      }
      @keyframes notify-slide-out {
        from { opacity: 1; transform: translateX(0); }
        to   { opacity: 0; transform: translateX(30px); }
      }

      /* Confirm modal */
      #notify-confirm-overlay {
        display: none;
        position: fixed;
        inset: 0;
        background: rgba(0,0,0,0.4);
        backdrop-filter: blur(4px);
        z-index: 99998;
        align-items: center;
        justify-content: center;
        animation: notify-fade-in 0.2s ease;
      }
      #notify-confirm-overlay.notify-visible {
        display: flex;
      }

      #notify-confirm-box {
        background: white;
        border-radius: 16px;
        padding: 28px 30px 22px;
        max-width: 380px;
        width: 90%;
        box-shadow: 0 20px 40px rgba(0,0,0,0.18);
        font-family: 'Inter', system-ui, sans-serif;
        animation: notify-pop-in 0.25s cubic-bezier(0.34,1.56,0.64,1);
      }

      #notify-confirm-icon { font-size: 32px; text-align: center; margin-bottom: 12px; }
      #notify-confirm-msg  { font-size: 15px; color: #202124; text-align: center; margin-bottom: 22px; line-height: 1.5; font-weight: 500; }

      #notify-confirm-btns {
        display: flex;
        gap: 10px;
        justify-content: center;
      }

      .notify-btn {
        padding: 10px 22px;
        border-radius: 8px;
        border: none;
        font-size: 14px;
        font-weight: 600;
        cursor: pointer;
        transition: all 0.15s ease;
        font-family: inherit;
      }
      .notify-btn:hover { opacity: 0.88; transform: translateY(-1px); }
      .notify-btn-cancel  { background: #f1f3f4; color: #5f6368; }
      .notify-btn-confirm { background: #1a73e8; color: white; }
      .notify-btn-danger  { background: #d93025; color: white; }

      @keyframes notify-fade-in {
        from { opacity: 0; } to { opacity: 1; }
      }
      @keyframes notify-pop-in {
        from { opacity: 0; transform: scale(0.9); }
        to   { opacity: 1; transform: scale(1); }
      }
    `;
    document.head.appendChild(style);
  }

  /* ── Toast container ── */
  let toastContainer = document.getElementById('notify-toast-container');
  if (!toastContainer) {
    toastContainer = document.createElement('div');
    toastContainer.id = 'notify-toast-container';
    document.body.appendChild(toastContainer);
  }

  /* ── Toast function ── */
  const icons = { success: '✅', error: '❌', warning: '⚠️', info: 'ℹ️' };

  window.toast = function (message, type = 'info', duration = 3500) {
    const el = document.createElement('div');
    el.className = `notify-toast notify-${type}`;
    el.innerHTML = `
      <span class="notify-icon">${icons[type] || icons.info}</span>
      <span class="notify-msg">${message}</span>
    `;
    toastContainer.appendChild(el);

    setTimeout(() => {
      el.classList.add('notify-hiding');
      el.addEventListener('animationend', () => el.remove());
    }, duration);
  };

  /* ── Confirm dialog (returns Promise<boolean>) ── */
  let overlay = document.getElementById('notify-confirm-overlay');
  if (!overlay) {
    overlay = document.createElement('div');
    overlay.id = 'notify-confirm-overlay';
    overlay.innerHTML = `
      <div id="notify-confirm-box">
        <div id="notify-confirm-icon"></div>
        <div id="notify-confirm-msg"></div>
        <div id="notify-confirm-btns">
          <button class="notify-btn notify-btn-cancel"  id="notify-btn-cancel">Cancelar</button>
          <button class="notify-btn notify-btn-confirm" id="notify-btn-ok">Confirmar</button>
        </div>
      </div>
    `;
    document.body.appendChild(overlay);
  }

  window.confirmDialog = function (message, { icon = '❓', danger = false, okLabel = 'Confirmar', cancelLabel = 'Cancelar' } = {}) {
    return new Promise((resolve) => {
      document.getElementById('notify-confirm-icon').textContent = icon;
      document.getElementById('notify-confirm-msg').textContent = message;

      const okBtn = document.getElementById('notify-btn-ok');
      const cancelBtn = document.getElementById('notify-btn-cancel');

      okBtn.textContent = okLabel;
      cancelBtn.textContent = cancelLabel;
      okBtn.className = `notify-btn ${danger ? 'notify-btn-danger' : 'notify-btn-confirm'}`;

      overlay.classList.add('notify-visible');

      const close = (result) => {
        overlay.classList.remove('notify-visible');
        okBtn.replaceWith(okBtn.cloneNode(true));
        cancelBtn.replaceWith(cancelBtn.cloneNode(true));
        resolve(result);
      };

      document.getElementById('notify-btn-ok').onclick     = () => close(true);
      document.getElementById('notify-btn-cancel').onclick = () => close(false);
    });
  };
})();