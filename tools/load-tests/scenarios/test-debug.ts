import { selectDepositWallet } from "../lib/data.ts";
import { submitDeposit, getJobStatus } from "../lib/api.ts";
import exec from "k6/execution";
import { sleep } from "k6";

export const options = {
  scenarios: {
    smoke_test: {
      executor: "shared-iterations",
      vus: 1,
      iterations: 1,
    },
  },
};

export default function (): void {
  const vu = exec.vu.idInTest;
  const iter = exec.scenario.iterationInTest;
  const { walletId, jwt } = selectDepositWallet(vu, iter);

  console.log("Submitting deposit for wallet:", walletId);
  const res = submitDeposit(walletId, 10000, jwt);
  console.log("Response status:", res.status);

  if (res.status === 202) {
    const body = res.json() as any;
    const jobId = body.jobId;
    console.log(`Deposit queued with jobId: ${jobId}. Polling status...`);

    let status = "pending";
    let attempts = 0;
    while (status !== "completed" && status !== "failed" && attempts < 10) {
      sleep(1);
      const jobRes = getJobStatus(jobId, jwt);
      if (jobRes.status === 200) {
        const jobBody = jobRes.json() as any;
        status = jobBody.status;
        console.log(`Attempt ${attempts + 1}: Job status is ${status}`);
      } else {
        console.log(
          `Attempt ${attempts + 1}: Failed to fetch job status: ${jobRes.status}`,
        );
      }
      attempts++;
    }

    if (status === "completed") {
      console.log("SUCCESS! Deposit processed end-to-end.");
    } else {
      console.log(`Finished polling. Final status: ${status}`);
    }
  }
}
