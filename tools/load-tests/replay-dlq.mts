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
const config: Config = JSON.parse(fs.readFileSync(configPath, "utf8")) as Config;
const BASE_URL: string = process.env.BASE_URL || config.baseUrl;
const dataPath: string = path.join(__dirname, "test-data.json");
const data: TestData = JSON.parse(fs.readFileSync(dataPath, "utf8")) as TestData;

async function replayAll(): Promise<void> {
  const apiKey = data.apiKeys[0];
  const authRes = await fetch(`${BASE_URL}/v1/auth/token`, {
    method: "POST",
    headers: { Authorization: `Bearer ${apiKey}` },
  });
  if (!authRes.ok) throw new Error(`Auth failed: ${authRes.status}`);
  const { token } = (await authRes.json()) as { token: string };

  console.log("Replaying webhook DLQ entries...");
  const res = await fetch(`${BASE_URL}/v1/admin/dlq/replay`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "X-RRQ-Edge": "true",
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ source: "webhook", limit: 10 }),
  });

  if (res.ok) {
    console.log("Webhook replay result:", await res.json());
  } else {
    console.error("Webhook replay error:", await res.text());
  }
}

replayAll().catch(console.error);
