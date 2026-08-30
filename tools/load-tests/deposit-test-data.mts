import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

interface Config {
  baseUrl: string;
}

interface TestData {
  jwts: string[];
  wallets: string[][];
  apiKeys: string[];
}

interface TokenResponse {
  token: string;
}

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ENV: string = process.env.ENV || "dev";
const configPath: string = path.join(__dirname, "config", `${ENV}.json`);
const config: Config = JSON.parse(
  fs.readFileSync(configPath, "utf8"),
) as Config;
const BASE_URL: string = process.env.BASE_URL || config.baseUrl;
const CONCURRENCY: number = 50; // Concurrent deposit requests
const INITIAL_DEPOSIT_AMOUNT: number = 10000000; // 100,000 NGN in minor units (kobo/cents)

const dataPath: string = path.join(__dirname, "test-data.json");
if (!fs.existsSync(dataPath)) {
  console.error("ERROR: test-data.json not found. Run './run.sh seed' first.");
  process.exit(1);
}

const data: TestData = JSON.parse(fs.readFileSync(dataPath, "utf8")) as TestData;

function uuidv4(): string {
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    const v = c === "x" ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

async function fetchWithRetry(
  url: string,
  options: RequestInit,
  retries: number = 3,
): Promise<Response> {
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

async function refreshToken(merchantIdx: number): Promise<string> {
  const apiKey = data.apiKeys[merchantIdx];
  const res = await fetchWithRetry(`${BASE_URL}/v1/auth/token`, {
    method: "POST",
    headers: { Authorization: `Bearer ${apiKey}` },
  });
  if (!res.ok) {
    throw new Error(`Failed to refresh token for merchant ${merchantIdx}: ${res.status}`);
  }
  const body = (await res.json()) as TokenResponse;
  data.jwts[merchantIdx] = body.token;
  return body.token;
}

async function main(): Promise<void> {
  const totalMerchants = data.wallets.length;
  let totalWallets = 0;
  for (const wList of data.wallets) {
    totalWallets += wList.length;
  }

  console.log("══════════════════════════════════════════════════════════════");
  console.log("  PRE-FUNDING / DEPOSITING TO WALLETS");
  console.log(`  Merchants:      ${totalMerchants}`);
  console.log(`  Total Wallets:  ${totalWallets}`);
  console.log(`  Deposit Amount: ${INITIAL_DEPOSIT_AMOUNT.toLocaleString()} NGN per wallet`);
  console.log(`  Concurrency:    ${CONCURRENCY}`);
  console.log(`  Target BaseURL: ${BASE_URL}`);
  console.log("══════════════════════════════════════════════════════════════\n");

  const depositTasks: (() => Promise<void>)[] = [];
  let completed = 0;
  let failed = 0;
  const startTime = Date.now();

  for (let m = 0; m < totalMerchants; m++) {
    const merchantIdx = m;
    const walletList = data.wallets[m];

    for (let w = 0; w < walletList.length; w++) {
      const walletId = walletList[w];
      const taskIndex = w;

      depositTasks.push(async () => {
        let jwt = data.jwts[merchantIdx];
        const idempotencyKey = `dep-${merchantIdx}-${taskIndex}-${uuidv4()}`;

        const sendDeposit = async (token: string) => {
          return fetchWithRetry(`${BASE_URL}/v1/transfers`, {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
              Authorization: `Bearer ${token}`,
              "X-Idempotency-Key": idempotencyKey,
            },
            body: JSON.stringify({
              from_wallet: "",
              to_wallet: walletId,
              amount: INITIAL_DEPOSIT_AMOUNT,
              currency: "NGN",
              reference: `seed-deposit-${merchantIdx}-${taskIndex}`,
            }),
          });
        };

        try {
          let res = await sendDeposit(jwt);
          if (res.status === 401) {
            // Token expired, refresh and retry once
            jwt = await refreshToken(merchantIdx);
            res = await sendDeposit(jwt);
          }

          if (res.status === 200 || res.status === 202) {
            completed++;
          } else {
            failed++;
            console.error(`\n[WARN] Deposit failed for wallet ${walletId}: HTTP ${res.status}`);
          }
        } catch (err) {
          failed++;
          console.error(`\n[ERROR] Deposit error for wallet ${walletId}:`, err);
        }

        const currentTotal = completed + failed;
        if (currentTotal % 200 === 0 || currentTotal === totalWallets) {
          const elapsed = (Date.now() - startTime) / 1000;
          const rate = (currentTotal / elapsed).toFixed(1);
          const percent = ((currentTotal / totalWallets) * 100).toFixed(1);
          process.stdout.write(
            `\rProgress: ${currentTotal}/${totalWallets} (${percent}%) | Speed: ${rate} req/s | Completed: ${completed} | Failed: ${failed}   `,
          );
        }
      });
    }
  }

  // Execute in batches of CONCURRENCY
  for (let i = 0; i < depositTasks.length; i += CONCURRENCY) {
    const batch = depositTasks.slice(i, i + CONCURRENCY);
    await Promise.all(batch.map((fn) => fn()));
  }

  console.log("\n\n══════════════════════════════════════════════════════════════");
  console.log("  PRE-FUNDING COMPLETE");
  console.log(`  Successfully Deposited: ${completed}/${totalWallets}`);
  console.log(`  Failed:                 ${failed}`);
  console.log(`  Duration:               ${((Date.now() - startTime) / 1000).toFixed(1)}s`);
  console.log("══════════════════════════════════════════════════════════════\n");
}

main().catch(console.error);
