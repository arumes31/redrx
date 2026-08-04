const encoder = new TextEncoder();

function hasLeadingZeroBits(bytes, bits) {
    let offset = 0;
    while (bits >= 8) {
        if (bytes[offset++] !== 0) return false;
        bits -= 8;
    }
    return bits === 0 || (bytes[offset] >> (8 - bits)) === 0;
}

self.addEventListener('message', async (event) => {
    const { challenge, difficulty } = event.data;
    let solution = 0;
    const started = performance.now();
    try {
        while (solution < Number.MAX_SAFE_INTEGER) {
            const digest = await crypto.subtle.digest('SHA-256', encoder.encode(`${challenge}:${solution}`));
            if (hasLeadingZeroBits(new Uint8Array(digest), difficulty)) {
                self.postMessage({ type: 'done', solution: String(solution), attempts: solution + 1 });
                return;
            }
            solution += 1;
            if (solution % 250 === 0) {
                self.postMessage({ type: 'progress', attempts: solution, elapsed: performance.now() - started });
            }
        }
    } catch (error) {
        self.postMessage({ type: 'error', message: error instanceof Error ? error.message : 'Verification failed' });
    }
});
