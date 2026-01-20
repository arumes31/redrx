// Canvas Animation & Cursor Trail

const initCanvas = () => {
    const canvas = document.getElementById('canvas');
    if (!canvas) return;

    const ctx = canvas.getContext('2d');
    let width, height;

    const resize = () => {
        width = canvas.width = window.innerWidth;
        height = canvas.height = window.innerHeight;
    };
    window.addEventListener('resize', resize);
    resize();

    // Check which animation to run based on a data attribute or class on the body
    const animType = document.body.dataset.animation || 'circles'; // default

    const circles = [];
    const dots = [];
    const maxParticles = 30;

    class Circle {
        constructor() {
            this.init();
        }
        init() {
            this.x = Math.random() * width;
            this.y = Math.random() * height;
            this.size = Math.random() * 10 + 2;
            this.color = `hsla(${Math.random() * 60 + 200}, 70%, 50%, ${Math.random() * 0.5 + 0.1})`;
            this.age = 0;
            this.lifespan = Math.random() * 200 + 100;
        }
        update() {
            this.age++;
            this.size += 0.05;
            if (this.age > this.lifespan) {
                this.init();
            }
        }
        draw() {
            ctx.beginPath();
            ctx.arc(this.x, this.y, this.size, 0, Math.PI * 2);
            ctx.fillStyle = this.color;
            ctx.fill();
        }
    }

    class Dot {
        constructor() {
           this.x = Math.random() * width;
           this.y = Math.random() * height;
           this.vx = (Math.random() - 0.5) * 1;
           this.vy = (Math.random() - 0.5) * 1;
           this.color = `rgba(100, 200, 255, ${Math.random() * 0.5})`; 
        }
        update() {
            this.x += this.vx;
            this.y += this.vy;
            if (this.x < 0 || this.x > width) this.vx *= -1;
            if (this.y < 0 || this.y > height) this.vy *= -1;
        }
        draw() {
            ctx.beginPath();
            ctx.arc(this.x, this.y, 2, 0, Math.PI * 2);
            ctx.fillStyle = this.color;
            ctx.fill();
        }
    }

    if (animType === 'circles') {
        for (let i = 0; i < maxParticles; i++) circles.push(new Circle());
    } else if (animType === 'dots') {
        for (let i = 0; i < 50; i++) dots.push(new Dot());
    }

    const animate = () => {
        ctx.clearRect(0, 0, width, height);
        
        if (animType === 'circles') {
            circles.forEach(c => {
                c.update();
                c.draw();
            });
        } else if (animType === 'dots') {
            dots.forEach(d => {
                d.update();
                d.draw();
            });
            // Draw lines
            for (let i = 0; i < dots.length; i++) {
                for (let j = i + 1; j < dots.length; j++) {
                    const dx = dots[i].x - dots[j].x;
                    const dy = dots[i].y - dots[j].y;
                    const dist = Math.sqrt(dx * dx + dy * dy);
                    if (dist < 150) {
                        ctx.beginPath();
                        ctx.moveTo(dots[i].x, dots[i].y);
                        ctx.lineTo(dots[j].x, dots[j].y);
                        ctx.strokeStyle = `rgba(100, 200, 255, ${1 - dist / 150})`;
                        ctx.stroke();
                    }
                }
            }
        }
        
        requestAnimationFrame(animate);
    };
    animate();
};

const initCursor = () => {
    const cursor = document.createElement('div');
    cursor.className = 'cursor-dot';
    document.body.appendChild(cursor);

    let mouseX = 0, mouseY = 0;
    let cursorX = 0, cursorY = 0;

    document.addEventListener('mousemove', (e) => {
        mouseX = e.clientX;
        mouseY = e.clientY;
    });

    const animateCursor = () => {
        // Smooth follow
        cursorX += (mouseX - cursorX) * 0.2;
        cursorY += (mouseY - cursorY) * 0.2;
        
        cursor.style.left = cursorX + 'px';
        cursor.style.top = cursorY + 'px';
        
        requestAnimationFrame(animateCursor);
    };
    animateCursor();
};

document.addEventListener('DOMContentLoaded', () => {
    initCanvas();
    initCursor();
});
