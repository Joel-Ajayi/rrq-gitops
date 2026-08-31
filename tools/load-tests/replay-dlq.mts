import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

interface Config {
  baseUrl: string;
}

interface TestData {
  apiKeys: string[];
}

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ENV: string = process.env.ENV || "dev";
const configPath: string = path.join(__dirname, "config", `${ENV}.json`);
const config: Config = JSON.parse(
  fs.readFileSync(configPath, "utf8"),
) as Config;
const BASE_URL: string = process.env.BASE_URL || config.baseUrl;
const dataPath: string = path.join(__dirname, "test-data.json");
const data: TestData = JSON.parse(
  fs.readFileSync(dataPath, "utf8"),
) as TestData;

async function replayAll(): Promise<void> {
  const apiKey = data.apiKeys[0];
  const authRes = await fetch(`${BASE_URL}/v1/auth/token`, {
    method: "POST",
    headers: { Authorization: `Bearer ${apiKey}` },
  });
  if (!authRes.ok) throw new Error(`Auth failed: ${authRes.status}`);
  const { token } = (await authRes.json()) as { token: string };

  console.log("Listing open DLQ entries...");
  const listRes = await fetch(
    `${BASE_URL}/v1/admin/dlq?limit=100&status=open`,
    {
      headers: {
        Authorization: `Bearer ${token}`,
        "X-RRQ-Edge": "true",
      },
    },
  );
  if (listRes.ok) {
    const list = await listRes.json();
    console.log(
      "Open DLQ entries found:",
      Array.isArray(list) ? list.length : list,
    );
    if (Array.isArray(list) && list.length > 0) {
      console.log(
        "Sample DLQ entries:",
        JSON.stringify(list.slice(0, 5), null, 2),
      );
    }
  }

  console.log("Replaying all open DLQ entries in batches...");
  let totalReplayed = 0;
  while (true) {
    const res = await fetch(`${BASE_URL}/v1/admin/dlq/replay`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${token}`,
        "X-RRQ-Edge": "true",
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ source: "", limit: 10 }),
    });

    if (!res.ok) {
      console.error("DLQ replay batch error:", await res.text());
      break;
    }

    const result = (await res.json()) as {
      replayed_count?: number;
      replayedCount?: number;
    };
    const count = result.replayed_count ?? result.replayedCount ?? 0;
    console.log(`Replayed batch: ${count} entries`);
    totalReplayed += count;
    if (count === 0) break;
  }

  console.log(`Total DLQ entries successfully replayed: ${totalReplayed}`);
}

replayAll().catch(console.error);
