const net = require("node:net");

const DEFAULT_SMTP_PORT = Number(process.env.HITKEEP_E2E_MAIL_PORT || 2525);

async function startSmtpCapture(port = DEFAULT_SMTP_PORT) {
    const messages = [];
    const waiters = [];
    const sockets = new Set();

    const server = net.createServer((socket) => {
        let buffer = "";
        let dataLines = [];
        let inData = false;

        sockets.add(socket);
        socket.once("close", () => sockets.delete(socket));
        socket.setEncoding("utf8");
        socket.write("220 hitkeep-e2e.local SMTP ready\r\n");

        socket.on("data", (chunk) => {
            buffer += chunk;

            while (buffer.includes("\n")) {
                const newline = buffer.indexOf("\n");
                const rawLine = buffer.slice(0, newline).replace(/\r$/, "");
                buffer = buffer.slice(newline + 1);

                if (inData) {
                    if (rawLine === ".") {
                        const message = dataLines.join("\n");
                        messages.push(message);
                        resolveWaiters(message);
                        dataLines = [];
                        inData = false;
                        socket.write("250 queued\r\n");
                    } else {
                        dataLines.push(rawLine.startsWith("..") ? rawLine.slice(1) : rawLine);
                    }
                    continue;
                }

                const command = rawLine.split(/\s+/, 1)[0].toUpperCase();
                switch (command) {
                    case "EHLO":
                    case "HELO":
                        socket.write("250-hitkeep-e2e.local\r\n250 8BITMIME\r\n");
                        break;
                    case "MAIL":
                    case "RCPT":
                    case "RSET":
                    case "NOOP":
                        socket.write("250 ok\r\n");
                        break;
                    case "DATA":
                        inData = true;
                        socket.write("354 end with <CRLF>.<CRLF>\r\n");
                        break;
                    case "QUIT":
                        socket.write("221 bye\r\n");
                        socket.end();
                        break;
                    default:
                        socket.write("250 ok\r\n");
                        break;
                }
            }
        });
    });

    await new Promise((resolve, reject) => {
        server.once("error", reject);
        server.listen(port, "127.0.0.1", () => {
            server.off("error", reject);
            resolve();
        });
    });

    function resolveWaiters(message) {
        for (let index = waiters.length - 1; index >= 0; index--) {
            const waiter = waiters[index];
            if (!waiter.predicate(message)) continue;

            clearTimeout(waiter.timer);
            waiters.splice(index, 1);
            waiter.resolve(message);
        }
    }

    async function waitForMessage(predicate = () => true, timeoutMs = 10_000) {
        const existing = messages.find(predicate);
        if (existing) return existing;

        return new Promise((resolve, reject) => {
            const timer = setTimeout(() => {
                const index = waiters.findIndex((waiter) => waiter.resolve === resolve);
                if (index >= 0) waiters.splice(index, 1);
                reject(new Error("Timed out waiting for captured email."));
            }, timeoutMs);
            waiters.push({ predicate, resolve, timer });
        });
    }

    async function close() {
        for (const socket of sockets) {
            socket.destroy();
        }
        await new Promise((resolve, reject) => {
            server.close((error) => (error ? reject(error) : resolve()));
        });
    }

    return { messages, waitForMessage, close };
}

function inviteLinkFromMessage(message) {
    const decoded = decodeQuotedPrintable(message);
    const match = decoded.match(/https?:\/\/[^\s<>"']+\/(?:login\?returnUrl=[^\s<>"']+|accept-invite\?token=[^\s<>"']+)/);
    if (!match) {
        throw new Error(`Captured email did not contain an invite link:\n${decoded}`);
    }
    return match[0].replace(/&amp;/g, "&");
}

function decodeQuotedPrintable(value) {
    return value.replace(/=\n/g, "").replace(/=([0-9A-F]{2})/gi, (_, hex) => String.fromCharCode(Number.parseInt(hex, 16)));
}

module.exports = {
    DEFAULT_SMTP_PORT,
    inviteLinkFromMessage,
    startSmtpCapture
};
