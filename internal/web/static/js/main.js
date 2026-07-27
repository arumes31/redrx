// Constellation Network Animation

const initCanvas = () => {
    const canvas = document.getElementById('canvas');
    if (!canvas) return;

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

document.addEventListener('DOMContentLoaded', () => {
    initCanvas();
});