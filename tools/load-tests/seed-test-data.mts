import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

interface Config {
  baseUrl: string;
}

interface MerchantResponse {
  merchantId: string;
  apiKey: string;
}

interface TokenResponse {
  token: string;
}

interface WalletResponse {
  walletId: string;
}

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ENV: string = process.env.ENV || "dev";
const configPath: string = path.join(__dirname, "config", `${ENV}.json`);
const config: Config = JSON.parse(fs.readFileSync(configPath, "utf8")) as Config;
const BASE_URL: string = process.env.BASE_URL || config.baseUrl;
const MERCHANT_COUNT: number = 100;
const WALLETS_PER_MERCHANT: number = 100;
const CONCURRENCY: number = 20; // Max concurrent requests per batch
const INITIAL_DEPOSIT_AMOUNT: number = 10000000; // 100,000 NGN in smallest unit

async function fetchWithRetry(url: string, options: RequestInit, retries: number = 3): Promise<Response> {
  for (let attempt = 0; attempt < retries; attempt++) {
    try {
      const res = await fetch(url, options);
      if (!res.ok && res.status >= 500 && attempt < retries - 1) {
        await new Promise((r) => setTimeout(r, 1000 * (attempt + 1)));
        continue;
      }
      return res;
    } catch (err) {
      if (attempt < retries - 1) {
        await new Promise((r) => setTimeout(r, 1000 * (attempt + 1)));
        continue;
      }
      throw err;
    }
  }
  throw new Error("Unreachable");
}

async function main(): Promise<void> {
  console.log(`\nPreparing test data: creating ${MERCHANT_COUNT} merchants...`);
  const merchants: { id: string; apiKey: string }[] = [];
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
    const body = (await res.json()) as MerchantResponse;
    merchants.push({ id: body.merchantId, apiKey: body.apiKey });
  }

  console.log(`Successfully created ${MERCHANT_COUNT} merchants. Acquiring JWTs...`);
  const jwts: string[] = [];
  for (let i = 0; i < MERCHANT_COUNT; i++) {
    const res = await fetchWithRetry(`${BASE_URL}/v1/auth/token`, {
      method: "POST",
      headers: { Authorization: `Bearer ${merchants[i].apiKey}` },
    });
    if (!res.ok) {
      console.error(`Failed to log in for merchant ${merchants[i].id}: status=${res.status}`);
      process.exit(1);
    }
    const body = (await res.json()) as TokenResponse;
    jwts.push(body.token);
  }

  console.log(`Creating ${MERCHANT_COUNT * WALLETS_PER_MERCHANT} wallets...`);
  const wallets: string[][] = Array.from({ length: MERCHANT_COUNT }, () => []);
  const ROUNDS = 10;
  const WALLETS_PER_ROUND = WALLETS_PER_MERCHANT / ROUNDS;

  for (let r = 0; r < ROUNDS; r++) {
    console.log(`  Wallet creation round ${r + 1}/${ROUNDS}...`);
    const requestFactories: (() => Promise<void>)[] = [];
    for (let m = 0; m < MERCHANT_COUNT; m++) {
      const jwt = jwts[m];
      for (let w = 0; w < WALLETS_PER_ROUND; w++) {
        requestFactories.push(() => 
          fetchWithRetry(`${BASE_URL}/v1/wallets`, {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
              Authorization: `Bearer ${jwt}`,
            },
            body: JSON.stringify({ currency: "NGN" }),
          }).then(async (res) => {
            if (!res.ok) throw new Error(`Wallet create failed: ${res.status}`);
            const body = (await res.json()) as WalletResponse;
            wallets[m].push(body.walletId);
          })
        );
      }
    }
    for (let i = 0; i < requestFactories.length; i += CONCURRENCY) {
      const batch = requestFactories.slice(i, i + CONCURRENCY);
      await Promise.all(batch.map(fn => fn()));
    }
  }

  console.log(`\nPre-funding ${MERCHANT_COUNT * WALLETS_PER_MERCHANT} wallets with initial deposits...`);
  let fundedCount = 0;
  for (let m = 0; m < MERCHANT_COUNT; m++) {
    const jwt = jwts[m];
    const depositFactories: (() => Promise<void>)[] = [];
    for (let w = 0; w < WALLETS_PER_MERCHANT; w++) {
      depositFactories.push(() => 
        fetchWithRetry(`${BASE_URL}/v1/transfers`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            Authorization: `Bearer ${jwt}`,
            "X-Idempotency-Key": `seed-dep-${m}-${w}`,
          },
          body: JSON.stringify({
            from_wallet: "",
            to_wallet: wallets[m][w],
            amount: INITIAL_DEPOSIT_AMOUNT,
            currency: "NGN",
            reference: `seed-ref-${m}-${w}`,
          }),
        }).then(async (res) => {
          if (!res.ok) {
            console.error(`Deposit failed for wallet ${wallets[m][w]}: status=${res.status}`);
          }
        })
      );
    }
    for (let i = 0; i < depositFactories.length; i += CONCURRENCY) {
      const batch = depositFactories.slice(i, i + CONCURRENCY);
      await Promise.all(batch.map(fn => fn()));
    }
    fundedCount += WALLETS_PER_MERCHANT;
    console.log(`  Funded ${fundedCount}/${MERCHANT_COUNT * WALLETS_PER_MERCHANT} wallets`);
  }

  console.log(`\nWaiting 30s for deposits to be processed by ledger worker...`);
  await new Promise((r) => setTimeout(r, 30000));

  console.log("Writing test-data.json...");
  const apiKeys = merchants.map((m) => m.apiKey);
  const outData = { jwts, wallets, apiKeys };
  fs.writeFileSync(
    path.join(__dirname, "test-data.json"),
    JSON.stringify(outData, null, 2),
  );
  console.log("Done! Written to load-tests/test-data.json");
  console.log(`  Merchants: ${MERCHANT_COUNT}`);
  console.log(`  Wallets: ${MERCHANT_COUNT * WALLETS_PER_MERCHANT}`);
  console.log(`  Initial deposit per wallet: ${INITIAL_DEPOSIT_AMOUNT}`);
}

main().catch(console.error);
