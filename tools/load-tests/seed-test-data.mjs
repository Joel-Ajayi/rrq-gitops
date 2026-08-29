import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ENV = process.env.ENV || "dev";
const configPath = path.join(__dirname, "config", `${ENV}.json`);
const config = JSON.parse(fs.readFileSync(configPath, "utf8"));
const BASE_URL = process.env.BASE_URL || config.baseUrl;
const MERCHANT_COUNT = 100;
const WALLETS_PER_MERCHANT = 100;
const CONCURRENCY = 20; // Max concurrent requests per batch
const INITIAL_DEPOSIT_AMOUNT = 10000000; // 100,000 NGN in smallest unit
async function fetchWithRetry(url, options, retries = 3) {
    for (let attempt = 0; attempt < retries; attempt++) {
        try {
            const res = await fetch(url, options);
            if (!res.ok && res.status >= 500 && attempt < retries - 1) {
                await new Promise((r) => setTimeout(r, 1000 * (attempt + 1)));
                continue;
            }
            return res;
        }
        catch (err) {
            if (attempt < retries - 1) {
                await new Promise((r) => setTimeout(r, 1000 * (attempt + 1)));
                continue;
            }
            throw err;
        }
    }
    throw new Error("Unreachable");
}
async function main() {
    console.log(`\nPreparing test data: creating ${MERCHANT_COUNT} merchants...`);
    const merchants = [];
    for (let i = 0; i < MERCHANT_COUNT; i++) {
        const res = await fetchWithRetry(`${BASE_URL}/v1/merchants`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                name: `Perf Merchant ${i + 1}`,
                webhookUrl: "http://webhook-echo:8080/",
                webhookSecret: `secret-key-perf-${i + 1}`,
                tier: "standard",
            }),
        });
        if (!res.ok) {
            console.error(`Failed to create merchant ${i}: status=${res.status}`);
            process.exit(1);
        }
        const body = (await res.json());
        merchants.push({ id: body.merchantId, apiKey: body.apiKey });
    }
    console.log(`Successfully created ${MERCHANT_COUNT} merchants. Acquiring JWTs...`);
    const jwts = [];
    for (let i = 0; i < MERCHANT_COUNT; i++) {
        const res = await fetchWithRetry(`${BASE_URL}/v1/auth/token`, {
            method: "POST",
            headers: { Authorization: `Bearer ${merchants[i].apiKey}` },
        });
        if (!res.ok) {
            console.error(`Failed to log in for merchant ${merchants[i].id}: status=${res.status}`);
            process.exit(1);
        }
        const body = (await res.json());
        jwts.push(body.token);
    }
    console.log(`Creating ${MERCHANT_COUNT * WALLETS_PER_MERCHANT} wallets...`);
    const wallets = Array.from({ length: MERCHANT_COUNT }, () => []);
    const ROUNDS = 10;
    const WALLETS_PER_ROUND = WALLETS_PER_MERCHANT / ROUNDS;
    for (let r = 0; r < ROUNDS; r++) {
        console.log(`  Wallet creation round ${r + 1}/${ROUNDS}...`);
        const requestFactories = [];
        for (let m = 0; m < MERCHANT_COUNT; m++) {
            const jwt = jwts[m];
            for (let w = 0; w < WALLETS_PER_ROUND; w++) {
                requestFactories.push(() => fetchWithRetry(`${BASE_URL}/v1/wallets`, {
                    method: "POST",
                    headers: {
                        "Content-Type": "application/json",
                        Authorization: `Bearer ${jwt}`,
                    },
                    body: JSON.stringify({ currency: "NGN" }),
                }).then(async (res) => {
                    if (!res.ok)
                        throw new Error(`Wallet create failed: ${res.status}`);
                    const body = (await res.json());
                    wallets[m].push(body.walletId);
                }));
            }
        }
        for (let i = 0; i < requestFactories.length; i += CONCURRENCY) {
            const batch = requestFactories.slice(i, i + CONCURRENCY);
            await Promise.all(batch.map(fn => fn()));
        }
    }
    console.log("Writing test-data.json...");
    const apiKeys = merchants.map((m) => m.apiKey);
    const outData = { jwts, wallets, apiKeys };
    fs.writeFileSync(path.join(__dirname, "test-data.json"), JSON.stringify(outData, null, 2));
    console.log("Done! Written to load-tests/test-data.json");
    console.log(`  Merchants: ${MERCHANT_COUNT}`);
    console.log(`  Wallets: ${MERCHANT_COUNT * WALLETS_PER_MERCHANT}`);
}
main().catch(console.error);
