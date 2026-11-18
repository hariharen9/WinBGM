// --- Animation & Interactivity System ---

function refreshIcons() {
  if (window.lucide && typeof window.lucide.createIcons === 'function') {
    try { window.lucide.createIcons(); } catch (e) {}
  }
}

function celebrateSuccess() {
  if (typeof window.confetti === 'function') {
    window.confetti({
      particleCount: 55,
      spread: 65,
      origin: { y: 0.78 },
      colors: ['#60a5fa', '#34d399', '#a78bfa', '#38bdf8', '#fbbf24']
    });
  }
}

function animateList(selector) {
  if (!window.anime) return;
  anime({
    targets: selector,
    opacity: [0, 1],
    translateY: [7, 0],
    delay: anime.stagger(15, { start: 20 }),
    duration: 280,
    easing: 'easeOutCubic'
  });
}

function animateCounters() {
  if (!window.anime) return;
  const A = apps(), svc = comps('service'), tasks = comps('task'), rev = A.filter(a => a.review);
  const items = [
    { el: '#stat-apps', val: A.length },
    { el: '#stat-svc', val: svc.length },
    { el: '#stat-tasks', val: tasks.length },
    { el: '#stat-rev', val: rev.length }
  ];
  items.forEach(item => {
    const el = document.querySelector(item.el);
    if (!el) return;
    const obj = { val: 0 };
    anime({
      targets: obj,
      val: item.val,
      round: 1,
      duration: 650,
      easing: 'easeOutQuart',
      update: () => { el.textContent = obj.val; }
    });
  });
}

function openModal() {
  const m = $('confirm-modal');
  m.classList.add('show');
  if (window.anime) {
    anime({
      targets: '#confirm-modal .modal',
      scale: [0.93, 1],
      opacity: [0, 1],
      duration: 220,
      easing: 'easeOutCubic'
    });
  }
}

function closeModal() {
  const m = $('confirm-modal');
  if (window.anime && m.classList.contains('show')) {
    anime({
      targets: '#confirm-modal .modal',
      scale: [1, 0.93],
      opacity: [1, 0],
      duration: 160,
      easing: 'easeInCubic',
      complete: () => {
        m.classList.remove('show');
        pending = null;
        if (inv) render();
      }
    });
  } else {
    m.classList.remove('show');
    pending = null;
    if (inv) render();
  }
}
