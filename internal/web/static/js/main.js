// Constellation Network Animation

const initCanvas = () => {
    const canvas = document.getElementById('canvas');
    if (!canvas) return;

    // Respect a reduced-motion preference: skip the particles, the interaction
    // and resize handlers, and the animation loop entirely. A full-screen 60fps
    // repaint is exactly what these visitors asked not to have.
    if (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
        return;
    }

    const ctx = canvas.getContext('2d');
    let width, height;

    // Resize handling
    const resize = () => {
        width = canvas.width = window.innerWidth;
        height = canvas.height = window.innerHeight;
    };
    window.addEventListener('resize', resize);
    resize();

    // Configuration
    const config = {
        particleColor: 'rgba(100, 200, 255, 0.5)',
        lineColor: 'rgba(100, 200, 255, 0.15)',
        particleAmount: Math.min(window.innerWidth / 15, 100), // Responsive count
        defaultSpeed: 0.5,
        variantSpeed: 0.5,
        linkRadius: 150,
    };

    let particles = [];
    const mouse = { x: -9999, y: -9999 }; // Off-screen default

    // Track mouse for interaction
    document.addEventListener('mousemove', (e) => {
        mouse.x = e.clientX;
        mouse.y = e.clientY;
    });

    // Particle Class
    class Particle {
        constructor() {
            this.x = Math.random() * width;
            this.y = Math.random() * height;
            this.vx = (Math.random() * config.variantSpeed) - (config.variantSpeed / 2);
            this.vy = (Math.random() * config.variantSpeed) - (config.variantSpeed / 2);
            this.size = Math.random() * 2 + 1;
        }

        update() {
            this.x += this.vx;
            this.y += this.vy;

            // Boundary wrap
            if (this.x < 0) this.x = width;
            if (this.x > width) this.x = 0;
            if (this.y < 0) this.y = height;
            if (this.y > height) this.y = 0;
        }

        draw() {
            ctx.beginPath();
            ctx.arc(this.x, this.y, this.size, 0, Math.PI * 2);
            ctx.fillStyle = config.particleColor;
            ctx.fill();
        }
    }

    // Initialize particles
    const initParticles = () => {
        particles = [];
        for (let i = 0; i < config.particleAmount; i++) {
            particles.push(new Particle());
        }
    };
    initParticles();

    // Animation Loop
    const animate = () => {
        ctx.clearRect(0, 0, width, height);

        // Update and draw particles
        for (let i = 0; i < particles.length; i++) {
            particles[i].update();
            particles[i].draw();

            // Link particles
            for (let j = i + 1; j < particles.length; j++) {
                const dx = particles[i].x - particles[j].x;
                const dy = particles[i].y - particles[j].y;
                const distance = Math.sqrt(dx * dx + dy * dy);

                if (distance < config.linkRadius) {
                    ctx.beginPath();
                    ctx.moveTo(particles[i].x, particles[i].y);
                    ctx.lineTo(particles[j].x, particles[j].y);
                    ctx.strokeStyle = config.lineColor;
                    ctx.lineWidth = 1 - (distance / config.linkRadius);
                    ctx.stroke();
                }
            }

            // Link to mouse
            const dx = particles[i].x - mouse.x;
            const dy = particles[i].y - mouse.y;
            const distance = Math.sqrt(dx * dx + dy * dy);

            if (distance < config.linkRadius + 50) {
                 ctx.beginPath();
                 ctx.moveTo(particles[i].x, particles[i].y);
                 ctx.lineTo(mouse.x, mouse.y);
                 ctx.strokeStyle = 'rgba(255, 255, 255, 0.2)'; // Brighter link to mouse
                 ctx.lineWidth = 1 - (distance / (config.linkRadius + 50));
                 ctx.stroke();
            }
        }

        requestAnimationFrame(animate);
    };

    animate();
};

// Copy any server-rendered flash messages into the aria-live region so they are
// announced to screen readers. The visible alerts render as before; the live
// region only exists to trigger the announcement, and populating it after load
// is what makes aria-live fire.
const announceFlashes = () => {
    const liveStatus = document.getElementById('liveStatus');
    if (!liveStatus) return;
    const flashes = document.querySelectorAll('#flashMessages [data-flash-message]');
    if (!flashes.length) return;
    const text = Array.from(flashes)
        .map((el) => el.getAttribute('data-flash-message').trim())
        .filter(Boolean)
        .join('. ');
    if (text) {
        setTimeout(() => { liveStatus.textContent = text; }, 200);
    }
};

const initAnonymousProof = () => {
    const form = document.getElementById('shortenForm');
    if (!form?.dataset.pow) return;

    const challenge = document.getElementById('powChallenge');
    const difficulty = document.getElementById('powDifficulty');
    const solution = document.getElementById('powSolution');
    const progress = document.getElementById('powProgress');
    const progressBar = document.getElementById('powProgressBar');
    const progressText = document.getElementById('powProgressText');
    const submit = document.getElementById('shortenSubmit');
    let running = false;

    form.addEventListener('submit', (event) => {
        if (solution.value || running) return;
        event.preventDefault();
        if (!form.reportValidity()) return;

        running = true;
        submit.disabled = true;
        progress.classList.remove('d-none');
        const worker = new Worker('/static/js/pow-worker.js');
        worker.addEventListener('message', ({ data }) => {
            if (data.type === 'progress') {
                const expected = 2 ** Number(difficulty.value);
                const percent = Math.min(99, Math.round(100 * (1 - Math.exp(-data.attempts / expected))));
                progressBar.style.width = `${percent}%`;
                progressText.textContent = `${percent}% · ${data.attempts.toLocaleString()} attempts`;
                return;
            }
            if (data.type === 'done') {
                progressBar.style.width = '100%';
                progressText.textContent = `Complete · ${data.attempts.toLocaleString()} attempts`;
                solution.value = data.solution;
                worker.terminate();
                form.submit();
                return;
            }
            worker.terminate();
            running = false;
            submit.disabled = false;
            progressText.textContent = 'Could not verify. Try again.';
        });
        worker.addEventListener('error', () => {
            worker.terminate();
            running = false;
            submit.disabled = false;
            progressText.textContent = 'Could not verify. Try again.';
        });
        worker.postMessage({ challenge: challenge.value, difficulty: Number(difficulty.value) });
    });
};

document.addEventListener('DOMContentLoaded', () => {
    initCanvas();
    announceFlashes();
    initAnonymousProof();
});
