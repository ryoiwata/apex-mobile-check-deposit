import { useState, useRef, useEffect, useCallback } from "react";

const ROLES = [
  { id: "investor", label: "Investor", icon: "👤" },
  { id: "vendor", label: "Vendor Service", icon: "🔍" },
  { id: "funding", label: "Funding Service", icon: "⚙️" },
  { id: "operator", label: "Operator", icon: "🛡️" },
  { id: "settlement", label: "Settlement", icon: "🏦" },
  { id: "overview", label: "Full System", icon: "🗺️" },
];

const NODE_TYPES = {
  start: { bg: "#0d1117", border: "#58a6ff", text: "#e6edf3", shape: "pill" },
  action: { bg: "#161b22", border: "#30363d", text: "#e6edf3", shape: "rect" },
  decision: { bg: "#1c1e26", border: "#d29922", text: "#e6edf3", shape: "diamond" },
  success: { bg: "#0d1117", border: "#3fb950", text: "#3fb950", shape: "rect" },
  error: { bg: "#0d1117", border: "#f85149", text: "#f85149", shape: "rect" },
  warning: { bg: "#0d1117", border: "#d29922", text: "#d29922", shape: "rect" },
  process: { bg: "#0d1117", border: "#bc8cff", text: "#bc8cff", shape: "rect" },
  end: { bg: "#0d1117", border: "#8b949e", text: "#8b949e", shape: "pill" },
  loop: { bg: "#0d1117", border: "#58a6ff", text: "#58a6ff", shape: "rect" },
};

const flowData = {
  investor: {
    title: "Investor Deposit Flow",
    subtitle: "Check submission, validation feedback, and status tracking",
    nodes: [
      { id: "start", type: "start", label: "Open Mobile App", x: 400, y: 40 },
      { id: "capture", type: "action", label: "Photograph Check\n(Front & Back)", x: 400, y: 130 },
      { id: "enter", type: "action", label: "Enter Deposit Amount\n& Select Account", x: 400, y: 220 },
      { id: "submit", type: "action", label: "Submit Deposit", x: 400, y: 310 },
      { id: "iqa_check", type: "decision", label: "Image Quality\nAcceptable?", x: 400, y: 420 },
      { id: "iqa_fail", type: "error", label: "IQA Error Displayed\n(Blur / Glare / etc.)", x: 140, y: 420 },
      { id: "retake", type: "loop", label: "Retake Photo\nWith Guidance", x: 140, y: 310 },
      { id: "validating", type: "process", label: "Status: Validating\n(Vendor Processing)", x: 400, y: 530 },
      { id: "biz_rules", type: "decision", label: "Business Rules\nPassed?", x: 400, y: 640 },
      { id: "all_violations", type: "error", label: "ALL Violations Shown\n(Collect-All Approach)", x: 140, y: 640 },
      { id: "fix_issues", type: "loop", label: "Fix All Issues\n& Resubmit", x: 140, y: 530 },
      { id: "approved", type: "success", label: "Status: Approved\nProvisional Credit", x: 400, y: 740 },
      { id: "funds_posted", type: "success", label: "Status: Funds Posted\nBalance Updated", x: 400, y: 830 },
      { id: "completed", type: "success", label: "Status: Completed\nSettlement Confirmed", x: 540, y: 920 },
      { id: "returned", type: "error", label: "Status: Returned\n$30 Fee Deducted", x: 260, y: 920 },
      { id: "return_notif", type: "warning", label: "Return Notification\nReceived", x: 260, y: 1010 },
      { id: "new_deposit", type: "loop", label: "Initiate New\nDeposit", x: 260, y: 1100 },
      { id: "end", type: "end", label: "Done", x: 540, y: 1010 },
    ],
    edges: [
      { from: "start", to: "capture", label: "" },
      { from: "capture", to: "enter", label: "" },
      { from: "enter", to: "submit", label: "" },
      { from: "submit", to: "iqa_check", label: "" },
      { from: "iqa_check", to: "iqa_fail", label: "No", type: "error" },
      { from: "iqa_fail", to: "retake", label: "" },
      { from: "retake", to: "capture", label: "Loop back", type: "loop" },
      { from: "iqa_check", to: "validating", label: "Yes", type: "success" },
      { from: "validating", to: "biz_rules", label: "" },
      { from: "biz_rules", to: "all_violations", label: "No", type: "error" },
      { from: "all_violations", to: "fix_issues", label: "Collect-all" },
      { from: "fix_issues", to: "enter", label: "Loop back", type: "loop" },
      { from: "biz_rules", to: "approved", label: "Yes", type: "success" },
      { from: "approved", to: "funds_posted", label: "" },
      { from: "funds_posted", to: "completed", label: "Settled" },
      { from: "funds_posted", to: "returned", label: "Bounced", type: "error" },
      { from: "returned", to: "return_notif", label: "" },
      { from: "return_notif", to: "new_deposit", label: "" },
      { from: "new_deposit", to: "capture", label: "Loop back", type: "loop" },
      { from: "completed", to: "end", label: "" },
    ],
  },
  vendor: {
    title: "Vendor Service Flow",
    subtitle: "Image quality assessment, MICR/OCR extraction, and duplicate detection",
    nodes: [
      { id: "start", type: "start", label: "Receive Deposit\nRequest", x: 400, y: 40 },
      { id: "iqa", type: "decision", label: "Image Quality\nAssessment (IQA)", x: 400, y: 150 },
      { id: "blur", type: "error", label: "IQA Fail: Blur\nPrompt Retake", x: 120, y: 100 },
      { id: "glare", type: "error", label: "IQA Fail: Glare\nPrompt Retake", x: 120, y: 200 },
      { id: "return_iqa", type: "loop", label: "Return IQA Error\nto Mobile App", x: 120, y: 310 },
      { id: "micr", type: "decision", label: "MICR Line\nReadable?", x: 400, y: 290 },
      { id: "micr_fail", type: "warning", label: "MICR Read Failure\nFlag for Review", x: 660, y: 290 },
      { id: "to_operator_micr", type: "process", label: "Route to Operator\nManual Review", x: 660, y: 400 },
      { id: "ocr", type: "action", label: "OCR Amount\nExtraction", x: 400, y: 400 },
      { id: "amt_match", type: "decision", label: "OCR Amount =\nEntered Amount?", x: 400, y: 520 },
      { id: "amt_mismatch", type: "warning", label: "Amount Mismatch\nFlag for Review", x: 660, y: 520 },
      { id: "to_operator_amt", type: "process", label: "Route to Operator\nAmount Verification", x: 660, y: 630 },
      { id: "dup", type: "decision", label: "Duplicate\nCheck?", x: 400, y: 640 },
      { id: "dup_yes", type: "error", label: "Duplicate Detected\nReject Deposit", x: 140, y: 640 },
      { id: "reject_return", type: "end", label: "Return Rejection\nto Funding Service", x: 140, y: 750 },
      { id: "clean", type: "success", label: "Clean Pass ✓\nAll Validations OK", x: 400, y: 760 },
      { id: "response", type: "action", label: "Return Structured Result\n(MICR data, amounts, txn ID)", x: 400, y: 870 },
      { id: "to_funding", type: "end", label: "Send to\nFunding Service", x: 400, y: 960 },
      { id: "await_re", type: "loop", label: "Await Resubmission\nfrom Investor", x: 120, y: 420 },
    ],
    edges: [
      { from: "start", to: "iqa", label: "" },
      { from: "iqa", to: "blur", label: "Blurry", type: "error" },
      { from: "iqa", to: "glare", label: "Glare", type: "error" },
      { from: "blur", to: "return_iqa", label: "" },
      { from: "glare", to: "return_iqa", label: "" },
      { from: "return_iqa", to: "await_re", label: "" },
      { from: "await_re", to: "start", label: "Loop back", type: "loop" },
      { from: "iqa", to: "micr", label: "Pass", type: "success" },
      { from: "micr", to: "micr_fail", label: "No", type: "error" },
      { from: "micr_fail", to: "to_operator_micr", label: "" },
      { from: "micr", to: "ocr", label: "Yes", type: "success" },
      { from: "ocr", to: "amt_match", label: "" },
      { from: "amt_match", to: "amt_mismatch", label: "No", type: "error" },
      { from: "amt_mismatch", to: "to_operator_amt", label: "" },
      { from: "amt_match", to: "dup", label: "Yes", type: "success" },
      { from: "dup", to: "dup_yes", label: "Yes", type: "error" },
      { from: "dup_yes", to: "reject_return", label: "" },
      { from: "dup", to: "clean", label: "No", type: "success" },
      { from: "clean", to: "response", label: "" },
      { from: "response", to: "to_funding", label: "" },
      { from: "to_operator_micr", to: "start", label: "After review loop", type: "loop" },
      { from: "to_operator_amt", to: "start", label: "After review loop", type: "loop" },
    ],
  },
  funding: {
    title: "Funding Service Flow",
    subtitle: "Business rules enforcement with collect-all validation approach",
    nodes: [
      { id: "start", type: "start", label: "Receive Validated\nDeposit", x: 400, y: 40 },
      { id: "session", type: "action", label: "Validate Investor\nSession & Auth", x: 400, y: 130 },
      { id: "session_ok", type: "decision", label: "Session\nValid?", x: 400, y: 240 },
      { id: "session_fail", type: "error", label: "Auth Failure\nReject Deposit", x: 140, y: 240 },
      { id: "reauth", type: "loop", label: "Prompt\nRe-authentication", x: 140, y: 130 },
      { id: "resolve", type: "action", label: "Resolve Account IDs\nto Internal Numbers", x: 400, y: 350 },
      { id: "collect_all", type: "process", label: "COLLECT-ALL\nRule Evaluation", x: 400, y: 460 },
      { id: "rule1", type: "action", label: "Rule 1: Deposit Limit\n(≤ $5,000)", x: 160, y: 560 },
      { id: "rule2", type: "action", label: "Rule 2: Contribution\nCap Check", x: 400, y: 560 },
      { id: "rule3", type: "action", label: "Rule 3: Duplicate\nDeposit Check", x: 640, y: 560 },
      { id: "aggregate", type: "decision", label: "Any Rules\nFailed?", x: 400, y: 670 },
      { id: "all_errors", type: "error", label: "Return ALL Violations\nat Once (Collect-All)", x: 140, y: 670 },
      { id: "fix_all", type: "loop", label: "Investor Fixes\nAll Issues", x: 140, y: 560 },
      { id: "contritype", type: "action", label: "Default Contribution\nType (Individual)", x: 400, y: 770 },
      { id: "create_xfer", type: "action", label: "Create Transfer Record\nMOVEMENT / DEPOSIT / CHECK", x: 400, y: 860 },
      { id: "ledger", type: "action", label: "Post to Ledger\n(From: Omnibus → To: Investor)", x: 400, y: 950 },
      { id: "state_approved", type: "success", label: "State → Approved\nFundsPosted", x: 400, y: 1040 },
      { id: "to_settlement", type: "end", label: "Queue for\nSettlement", x: 400, y: 1130 },
    ],
    edges: [
      { from: "start", to: "session", label: "" },
      { from: "session", to: "session_ok", label: "" },
      { from: "session_ok", to: "session_fail", label: "No", type: "error" },
      { from: "session_fail", to: "reauth", label: "" },
      { from: "reauth", to: "start", label: "Loop back", type: "loop" },
      { from: "session_ok", to: "resolve", label: "Yes", type: "success" },
      { from: "resolve", to: "collect_all", label: "" },
      { from: "collect_all", to: "rule1", label: "Evaluate all" },
      { from: "collect_all", to: "rule2", label: "Evaluate all" },
      { from: "collect_all", to: "rule3", label: "Evaluate all" },
      { from: "rule1", to: "aggregate", label: "" },
      { from: "rule2", to: "aggregate", label: "" },
      { from: "rule3", to: "aggregate", label: "" },
      { from: "aggregate", to: "all_errors", label: "Yes", type: "error" },
      { from: "all_errors", to: "fix_all", label: "" },
      { from: "fix_all", to: "start", label: "Loop back", type: "loop" },
      { from: "aggregate", to: "contritype", label: "No (all pass)", type: "success" },
      { from: "contritype", to: "create_xfer", label: "" },
      { from: "create_xfer", to: "ledger", label: "" },
      { from: "ledger", to: "state_approved", label: "" },
      { from: "state_approved", to: "to_settlement", label: "" },
    ],
  },
  operator: {
    title: "Operator Review Flow",
    subtitle: "Manual review queue, approve/reject workflow, and audit trail",
    nodes: [
      { id: "start", type: "start", label: "Flagged Deposit\nEnters Queue", x: 400, y: 40 },
      { id: "queue", type: "action", label: "Review Queue\n(Filter: Date/Status/Amount)", x: 400, y: 140 },
      { id: "select", type: "action", label: "Select Flagged Item\nView Details", x: 400, y: 240 },
      { id: "review", type: "action", label: "Review Check Images\nMICR Data & Scores", x: 400, y: 340 },
      { id: "compare", type: "action", label: "Compare Recognized\nvs. Entered Amount", x: 400, y: 440 },
      { id: "decision", type: "decision", label: "Approve or\nReject?", x: 400, y: 560 },
      { id: "override", type: "decision", label: "Override\nContribution Type?", x: 600, y: 460 },
      { id: "do_override", type: "action", label: "Set Contribution\nType Override", x: 740, y: 560 },
      { id: "approve", type: "success", label: "Approve Deposit\nLog Action", x: 600, y: 660 },
      { id: "reject", type: "error", label: "Reject Deposit\nLog Reason", x: 200, y: 660 },
      { id: "audit", type: "action", label: "Audit Log Entry\n(Who, What, When)", x: 400, y: 760 },
      { id: "notify_approve", type: "success", label: "Investor Notified\n→ Proceeds to Settlement", x: 600, y: 860 },
      { id: "notify_reject", type: "warning", label: "Investor Notified\n→ May Resubmit", x: 200, y: 860 },
      { id: "resubmit", type: "loop", label: "Investor Resubmits\nNew Deposit", x: 200, y: 960 },
      { id: "next", type: "loop", label: "Next Item\nin Queue", x: 600, y: 960 },
      { id: "more", type: "decision", label: "More Items\nin Queue?", x: 400, y: 1060 },
      { id: "end", type: "end", label: "Queue Empty\nSession End", x: 400, y: 1160 },
    ],
    edges: [
      { from: "start", to: "queue", label: "" },
      { from: "queue", to: "select", label: "" },
      { from: "select", to: "review", label: "" },
      { from: "review", to: "compare", label: "" },
      { from: "compare", to: "decision", label: "" },
      { from: "compare", to: "override", label: "Optional" },
      { from: "override", to: "do_override", label: "Yes" },
      { from: "do_override", to: "decision", label: "" },
      { from: "override", to: "decision", label: "No" },
      { from: "decision", to: "approve", label: "Approve", type: "success" },
      { from: "decision", to: "reject", label: "Reject", type: "error" },
      { from: "approve", to: "audit", label: "" },
      { from: "reject", to: "audit", label: "" },
      { from: "audit", to: "notify_approve", label: "If approved" },
      { from: "audit", to: "notify_reject", label: "If rejected" },
      { from: "notify_reject", to: "resubmit", label: "" },
      { from: "resubmit", to: "start", label: "Loop back", type: "loop" },
      { from: "notify_approve", to: "next", label: "" },
      { from: "next", to: "more", label: "" },
      { from: "more", to: "queue", label: "Yes", type: "loop" },
      { from: "more", to: "end", label: "No" },
    ],
  },
  settlement: {
    title: "Settlement & Returns Flow",
    subtitle: "X9 ICL file generation, EOD processing, and return/reversal handling",
    nodes: [
      { id: "start", type: "start", label: "Approved Deposits\nQueued", x: 400, y: 40 },
      { id: "cutoff", type: "decision", label: "Before 6:30 PM\nCT Cutoff?", x: 400, y: 150 },
      { id: "next_day", type: "warning", label: "Roll to Next\nBusiness Day", x: 140, y: 150 },
      { id: "wait", type: "loop", label: "Hold Until Next\nBusiness Day EOD", x: 140, y: 260 },
      { id: "batch", type: "action", label: "Batch Deposits\nfor Settlement", x: 400, y: 270 },
      { id: "x9", type: "action", label: "Generate X9 ICL File\n(MICR + Images + Metadata)", x: 400, y: 370 },
      { id: "submit_bank", type: "action", label: "Submit to\nSettlement Bank", x: 400, y: 470 },
      { id: "ack", type: "decision", label: "Bank\nAcknowledged?", x: 400, y: 580 },
      { id: "ack_fail", type: "warning", label: "No Acknowledgment\nMonitor & Retry", x: 660, y: 580 },
      { id: "retry", type: "loop", label: "Retry Submission\nor Alert Ops", x: 660, y: 470 },
      { id: "settled", type: "success", label: "State → Completed\nSettlement Confirmed", x: 400, y: 690 },
      { id: "monitor", type: "action", label: "Monitor for\nReturns/Bounces", x: 400, y: 790 },
      { id: "returned", type: "decision", label: "Check\nReturned?", x: 400, y: 900 },
      { id: "no_return", type: "end", label: "Deposit Finalized\nNo Further Action", x: 600, y: 1000 },
      { id: "reversal", type: "error", label: "Create Reversal\nPosting", x: 200, y: 1000 },
      { id: "fee", type: "action", label: "Deduct $30\nReturn Fee", x: 200, y: 1100 },
      { id: "state_returned", type: "error", label: "State → Returned\nNotify Investor", x: 200, y: 1200 },
      { id: "new_deposit", type: "loop", label: "Investor May\nResubmit New Deposit", x: 200, y: 1300 },
    ],
    edges: [
      { from: "start", to: "cutoff", label: "" },
      { from: "cutoff", to: "next_day", label: "After", type: "error" },
      { from: "next_day", to: "wait", label: "" },
      { from: "wait", to: "cutoff", label: "Loop back", type: "loop" },
      { from: "cutoff", to: "batch", label: "Before", type: "success" },
      { from: "batch", to: "x9", label: "" },
      { from: "x9", to: "submit_bank", label: "" },
      { from: "submit_bank", to: "ack", label: "" },
      { from: "ack", to: "ack_fail", label: "No", type: "error" },
      { from: "ack_fail", to: "retry", label: "" },
      { from: "retry", to: "submit_bank", label: "Loop back", type: "loop" },
      { from: "ack", to: "settled", label: "Yes", type: "success" },
      { from: "settled", to: "monitor", label: "" },
      { from: "monitor", to: "returned", label: "" },
      { from: "returned", to: "no_return", label: "No", type: "success" },
      { from: "returned", to: "reversal", label: "Yes", type: "error" },
      { from: "reversal", to: "fee", label: "" },
      { from: "fee", to: "state_returned", label: "" },
      { from: "state_returned", to: "new_deposit", label: "" },
      { from: "new_deposit", to: "start", label: "Loop back", type: "loop" },
    ],
  },
  overview: {
    title: "Full System Overview",
    subtitle: "End-to-end deposit lifecycle across all services",
    nodes: [
      { id: "investor", type: "start", label: "📱 Investor\nSubmits Check", x: 400, y: 40 },
      { id: "vendor_iqa", type: "decision", label: "🔍 Vendor: IQA\nImage Quality?", x: 400, y: 160 },
      { id: "iqa_fail", type: "error", label: "IQA Failure\n(Blur/Glare)", x: 120, y: 160 },
      { id: "retake", type: "loop", label: "Investor\nRetakes Photo", x: 120, y: 60 },
      { id: "vendor_micr", type: "decision", label: "🔍 Vendor: MICR\nReadable?", x: 400, y: 280 },
      { id: "micr_fail", type: "warning", label: "MICR Failure\n→ Operator Queue", x: 680, y: 280 },
      { id: "vendor_ocr", type: "decision", label: "🔍 Vendor: Amount\nMatch?", x: 400, y: 400 },
      { id: "amt_fail", type: "warning", label: "Mismatch\n→ Operator Queue", x: 680, y: 400 },
      { id: "vendor_dup", type: "decision", label: "🔍 Vendor: Duplicate\nCheck?", x: 400, y: 520 },
      { id: "dup_reject", type: "error", label: "Duplicate → Reject", x: 120, y: 520 },
      { id: "operator", type: "process", label: "🛡️ Operator\nReview Queue", x: 680, y: 520 },
      { id: "op_decision", type: "decision", label: "🛡️ Approve\nor Reject?", x: 680, y: 640 },
      { id: "op_reject", type: "error", label: "Operator Rejects\nNotify Investor", x: 820, y: 740 },
      { id: "funding", type: "process", label: "⚙️ Funding Service\nCollect-All Rules", x: 400, y: 640 },
      { id: "rules_pass", type: "decision", label: "⚙️ All Rules\nPassed?", x: 400, y: 760 },
      { id: "violations", type: "error", label: "ALL Violations\nReturned at Once", x: 120, y: 760 },
      { id: "fix_resubmit", type: "loop", label: "Investor Fixes\nAll & Resubmits", x: 120, y: 640 },
      { id: "ledger", type: "success", label: "⚙️ Ledger Posted\nFunds Provisional", x: 400, y: 880 },
      { id: "settlement", type: "action", label: "🏦 X9 File Generated\nEOD Batch Submit", x: 400, y: 990 },
      { id: "completed", type: "success", label: "✅ Completed\nSettlement Confirmed", x: 540, y: 1100 },
      { id: "returned", type: "error", label: "❌ Returned\nReversal + $30 Fee", x: 260, y: 1100 },
      { id: "return_loop", type: "loop", label: "Investor May\nStart New Deposit", x: 260, y: 1200 },
      { id: "resubmit_dup", type: "loop", label: "Investor Submits\nDifferent Check", x: 120, y: 420 },
      { id: "op_rej_loop", type: "loop", label: "Investor May\nResubmit", x: 820, y: 840 },
    ],
    edges: [
      { from: "investor", to: "vendor_iqa", label: "" },
      { from: "vendor_iqa", to: "iqa_fail", label: "Fail", type: "error" },
      { from: "iqa_fail", to: "retake", label: "" },
      { from: "retake", to: "investor", label: "Loop", type: "loop" },
      { from: "vendor_iqa", to: "vendor_micr", label: "Pass", type: "success" },
      { from: "vendor_micr", to: "micr_fail", label: "Fail", type: "error" },
      { from: "micr_fail", to: "operator", label: "" },
      { from: "vendor_micr", to: "vendor_ocr", label: "Pass", type: "success" },
      { from: "vendor_ocr", to: "amt_fail", label: "Mismatch", type: "error" },
      { from: "amt_fail", to: "operator", label: "" },
      { from: "vendor_ocr", to: "vendor_dup", label: "Match", type: "success" },
      { from: "vendor_dup", to: "dup_reject", label: "Yes", type: "error" },
      { from: "dup_reject", to: "resubmit_dup", label: "" },
      { from: "resubmit_dup", to: "investor", label: "Loop", type: "loop" },
      { from: "vendor_dup", to: "funding", label: "No", type: "success" },
      { from: "operator", to: "op_decision", label: "" },
      { from: "op_decision", to: "funding", label: "Approve", type: "success" },
      { from: "op_decision", to: "op_reject", label: "Reject", type: "error" },
      { from: "op_reject", to: "op_rej_loop", label: "" },
      { from: "op_rej_loop", to: "investor", label: "Loop", type: "loop" },
      { from: "funding", to: "rules_pass", label: "" },
      { from: "rules_pass", to: "violations", label: "No", type: "error" },
      { from: "violations", to: "fix_resubmit", label: "Collect-all" },
      { from: "fix_resubmit", to: "investor", label: "Loop", type: "loop" },
      { from: "rules_pass", to: "ledger", label: "Yes", type: "success" },
      { from: "ledger", to: "settlement", label: "" },
      { from: "settlement", to: "completed", label: "Settled" },
      { from: "settlement", to: "returned", label: "Bounced", type: "error" },
      { from: "returned", to: "return_loop", label: "" },
      { from: "return_loop", to: "investor", label: "Loop", type: "loop" },
    ],
  },
};

function getNodeCenter(node, type) {
  const w = type === "diamond" ? 80 : 90;
  const h = type === "diamond" ? 40 : 28;
  return { cx: node.x, cy: node.y + h };
}

function FlowDiagram({ data, activeNode, setActiveNode }) {
  const svgRef = useRef(null);
  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const [dragging, setDragging] = useState(false);
  const [dragStart, setDragStart] = useState({ x: 0, y: 0 });

  const nodeMap = {};
  data.nodes.forEach((n) => (nodeMap[n.id] = n));

  const maxY = Math.max(...data.nodes.map((n) => n.y)) + 100;
  const maxX = Math.max(...data.nodes.map((n) => n.x)) + 200;
  const viewWidth = Math.max(maxX, 860);
  const viewHeight = maxY + 40;

  const handleWheel = useCallback((e) => {
    e.preventDefault();
    const delta = e.deltaY > 0 ? 0.9 : 1.1;
    setZoom((z) => Math.min(Math.max(z * delta, 0.3), 3));
  }, []);

  useEffect(() => {
    const svg = svgRef.current;
    if (svg) {
      svg.addEventListener("wheel", handleWheel, { passive: false });
      return () => svg.removeEventListener("wheel", handleWheel);
    }
  }, [handleWheel]);

  const handleMouseDown = (e) => {
    if (e.target.closest(".flow-node")) return;
    setDragging(true);
    setDragStart({ x: e.clientX - pan.x, y: e.clientY - pan.y });
  };
  const handleMouseMove = (e) => {
    if (!dragging) return;
    setPan({ x: e.clientX - dragStart.x, y: e.clientY - dragStart.y });
  };
  const handleMouseUp = () => setDragging(false);

  function renderEdge(edge, i) {
    const fromNode = nodeMap[edge.from];
    const toNode = nodeMap[edge.to];
    if (!fromNode || !toNode) return null;

    const fromType = NODE_TYPES[fromNode.type];
    const toType = NODE_TYPES[toNode.type];
    const fc = getNodeCenter(fromNode, fromType.shape);
    const tc = getNodeCenter(toNode, toType.shape);

    let color = "#30363d";
    if (edge.type === "success") color = "#3fb950";
    if (edge.type === "error") color = "#f85149";
    if (edge.type === "loop") color = "#58a6ff";

    const isLoop = edge.type === "loop";
    const dx = tc.cx - fc.cx;
    const dy = tc.cy - fc.cy;

    let path;
    if (isLoop && Math.abs(dy) > 200) {
      const goLeft = fc.cx >= tc.cx;
      const offsetX = goLeft ? -60 : 60;
      path = `M ${fc.cx} ${fc.cy} C ${fc.cx + offsetX} ${fc.cy}, ${tc.cx + offsetX} ${tc.cy}, ${tc.cx} ${tc.cy}`;
    } else if (Math.abs(dx) > 20 && Math.abs(dy) > 20) {
      const midY = fc.cy + (tc.cy - fc.cy) * 0.5;
      path = `M ${fc.cx} ${fc.cy} C ${fc.cx} ${midY}, ${tc.cx} ${midY}, ${tc.cx} ${tc.cy}`;
    } else {
      path = `M ${fc.cx} ${fc.cy} L ${tc.cx} ${tc.cy}`;
    }

    const midX = (fc.cx + tc.cx) / 2;
    const midY = (fc.cy + tc.cy) / 2;

    return (
      <g key={`e-${i}`}>
        <path
          d={path}
          fill="none"
          stroke={color}
          strokeWidth={isLoop ? 1.5 : 1.2}
          strokeDasharray={isLoop ? "6 3" : "none"}
          opacity={0.7}
          markerEnd={`url(#arrow-${edge.type || "default"})`}
        />
        {edge.label && (
          <text
            x={midX}
            y={midY - 6}
            textAnchor="middle"
            fill={color}
            fontSize="9"
            fontWeight="600"
            fontFamily="'JetBrains Mono', monospace"
          >
            {edge.label}
          </text>
        )}
      </g>
    );
  }

  function renderNode(node) {
    const type = NODE_TYPES[node.type];
    const isActive = activeNode === node.id;
    const lines = node.label.split("\n");
    const w = 160;
    const h = 24 + lines.length * 16;

    let shape;
    if (type.shape === "diamond") {
      const dw = 100;
      const dh = 26 + lines.length * 14;
      shape = (
        <polygon
          points={`${node.x},${node.y + 4} ${node.x + dw},${node.y + dh} ${node.x},${node.y + dh * 2 - 4} ${node.x - dw},${node.y + dh}`}
          fill={type.bg}
          stroke={isActive ? "#fff" : type.border}
          strokeWidth={isActive ? 2 : 1.2}
          rx="4"
        />
      );
    } else if (type.shape === "pill") {
      shape = (
        <rect
          x={node.x - w / 2}
          y={node.y}
          width={w}
          height={h}
          rx={h / 2}
          fill={type.bg}
          stroke={isActive ? "#fff" : type.border}
          strokeWidth={isActive ? 2 : 1.2}
        />
      );
    } else {
      shape = (
        <rect
          x={node.x - w / 2}
          y={node.y}
          width={w}
          height={h}
          rx="6"
          fill={type.bg}
          stroke={isActive ? "#fff" : type.border}
          strokeWidth={isActive ? 2 : 1.2}
        />
      );
    }

    return (
      <g
        key={node.id}
        className="flow-node"
        style={{ cursor: "pointer" }}
        onClick={() => setActiveNode(isActive ? null : node.id)}
      >
        {shape}
        {lines.map((line, li) => (
          <text
            key={li}
            x={node.x}
            y={node.y + 18 + li * 15 + (type.shape === "diamond" ? (lines.length > 1 ? 8 : 12) : 0)}
            textAnchor="middle"
            fill={isActive ? "#fff" : type.text}
            fontSize="10.5"
            fontFamily="'JetBrains Mono', monospace"
            fontWeight={li === 0 ? "600" : "400"}
          >
            {line}
          </text>
        ))}
      </g>
    );
  }

  return (
    <div
      style={{
        width: "100%",
        height: "100%",
        overflow: "hidden",
        position: "relative",
        background: "#010409",
      }}
      onMouseDown={handleMouseDown}
      onMouseMove={handleMouseMove}
      onMouseUp={handleMouseUp}
      onMouseLeave={handleMouseUp}
    >
      <svg
        ref={svgRef}
        width="100%"
        height="100%"
        viewBox={`0 0 ${viewWidth} ${viewHeight}`}
        style={{
          transform: `scale(${zoom}) translate(${pan.x / zoom}px, ${pan.y / zoom}px)`,
          transformOrigin: "0 0",
          cursor: dragging ? "grabbing" : "grab",
        }}
      >
        <defs>
          {["default", "success", "error", "loop"].map((t) => {
            const colors = {
              default: "#30363d",
              success: "#3fb950",
              error: "#f85149",
              loop: "#58a6ff",
            };
            return (
              <marker
                key={t}
                id={`arrow-${t}`}
                viewBox="0 0 10 10"
                refX="9"
                refY="5"
                markerWidth="7"
                markerHeight="7"
                orient="auto-start-reverse"
              >
                <path d="M 0 0 L 10 5 L 0 10 z" fill={colors[t]} />
              </marker>
            );
          })}
        </defs>
        {data.edges.map((e, i) => renderEdge(e, i))}
        {data.nodes.map((n) => renderNode(n))}
      </svg>
      <div
        style={{
          position: "absolute",
          bottom: 12,
          right: 12,
          display: "flex",
          gap: 4,
        }}
      >
        <button
          onClick={() => setZoom((z) => Math.min(z * 1.2, 3))}
          style={zoomBtnStyle}
        >
          +
        </button>
        <button
          onClick={() => setZoom((z) => Math.max(z * 0.8, 0.3))}
          style={zoomBtnStyle}
        >
          −
        </button>
        <button
          onClick={() => { setZoom(1); setPan({ x: 0, y: 0 }); }}
          style={{ ...zoomBtnStyle, width: "auto", padding: "0 8px", fontSize: 10 }}
        >
          Reset
        </button>
      </div>
    </div>
  );
}

const zoomBtnStyle = {
  width: 28,
  height: 28,
  background: "#161b22",
  border: "1px solid #30363d",
  color: "#e6edf3",
  borderRadius: 4,
  cursor: "pointer",
  fontSize: 14,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  fontFamily: "'JetBrains Mono', monospace",
};

function NodeDetail({ node, edges, nodeMap }) {
  if (!node) return null;
  const type = NODE_TYPES[node.type];
  const inEdges = edges.filter((e) => e.to === node.id);
  const outEdges = edges.filter((e) => e.from === node.id);

  return (
    <div
      style={{
        background: "#0d1117",
        border: `1px solid ${type.border}`,
        borderRadius: 8,
        padding: 16,
        marginTop: 12,
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 10 }}>
        <span
          style={{
            width: 10,
            height: 10,
            borderRadius: "50%",
            background: type.border,
            display: "inline-block",
          }}
        />
        <span style={{ color: type.text, fontWeight: 700, fontSize: 14 }}>
          {node.label.replace("\n", " ")}
        </span>
        <span
          style={{
            fontSize: 10,
            color: "#8b949e",
            background: "#161b22",
            padding: "2px 8px",
            borderRadius: 10,
            marginLeft: "auto",
          }}
        >
          {node.type.toUpperCase()}
        </span>
      </div>
      {inEdges.length > 0 && (
        <div style={{ marginBottom: 8 }}>
          <span style={{ color: "#8b949e", fontSize: 10, fontWeight: 600 }}>INCOMING FROM:</span>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 4, marginTop: 4 }}>
            {inEdges.map((e, i) => (
              <span
                key={i}
                style={{
                  fontSize: 10,
                  color: "#58a6ff",
                  background: "#161b22",
                  padding: "2px 8px",
                  borderRadius: 4,
                  border: "1px solid #21262d",
                }}
              >
                {nodeMap[e.from]?.label.split("\n")[0]} {e.label ? `(${e.label})` : ""}
              </span>
            ))}
          </div>
        </div>
      )}
      {outEdges.length > 0 && (
        <div>
          <span style={{ color: "#8b949e", fontSize: 10, fontWeight: 600 }}>OUTGOING TO:</span>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 4, marginTop: 4 }}>
            {outEdges.map((e, i) => {
              const c = e.type === "success" ? "#3fb950" : e.type === "error" ? "#f85149" : e.type === "loop" ? "#58a6ff" : "#8b949e";
              return (
                <span
                  key={i}
                  style={{
                    fontSize: 10,
                    color: c,
                    background: "#161b22",
                    padding: "2px 8px",
                    borderRadius: 4,
                    border: `1px solid ${c}33`,
                  }}
                >
                  {nodeMap[e.to]?.label.split("\n")[0]} {e.label ? `(${e.label})` : ""}
                </span>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}

function Legend() {
  const items = [
    { color: "#58a6ff", shape: "pill", label: "Start / Loop-back" },
    { color: "#30363d", shape: "rect", label: "Action / Process" },
    { color: "#d29922", shape: "diamond", label: "Decision Point" },
    { color: "#3fb950", shape: "rect", label: "Success State" },
    { color: "#f85149", shape: "rect", label: "Error / Rejection" },
    { color: "#bc8cff", shape: "rect", label: "Processing" },
    { color: "#8b949e", shape: "pill", label: "End / Terminal" },
  ];
  const edgeItems = [
    { color: "#3fb950", dash: false, label: "Happy path" },
    { color: "#f85149", dash: false, label: "Error path" },
    { color: "#58a6ff", dash: true, label: "Loop-back" },
    { color: "#30363d", dash: false, label: "Standard flow" },
  ];

  return (
    <div style={{ display: "flex", flexWrap: "wrap", gap: 16, padding: "8px 0" }}>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
        {items.map((it) => (
          <div key={it.label} style={{ display: "flex", alignItems: "center", gap: 5 }}>
            <span
              style={{
                width: 12,
                height: 12,
                border: `1.5px solid ${it.color}`,
                borderRadius: it.shape === "pill" ? 6 : it.shape === "diamond" ? 2 : 3,
                background: "#0d1117",
                transform: it.shape === "diamond" ? "rotate(45deg)" : "none",
                display: "inline-block",
              }}
            />
            <span style={{ fontSize: 10, color: "#8b949e" }}>{it.label}</span>
          </div>
        ))}
      </div>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
        {edgeItems.map((it) => (
          <div key={it.label} style={{ display: "flex", alignItems: "center", gap: 5 }}>
            <svg width="20" height="6">
              <line
                x1="0" y1="3" x2="20" y2="3"
                stroke={it.color}
                strokeWidth="1.5"
                strokeDasharray={it.dash ? "4 2" : "none"}
              />
            </svg>
            <span style={{ fontSize: 10, color: "#8b949e" }}>{it.label}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

export default function App() {
  const [activeRole, setActiveRole] = useState("overview");
  const [activeNode, setActiveNode] = useState(null);
  const data = flowData[activeRole];
  const nodeMap = {};
  data.nodes.forEach((n) => (nodeMap[n.id] = n));
  const selectedNode = activeNode ? nodeMap[activeNode] : null;

  return (
    <div
      style={{
        width: "100%",
        minHeight: "100vh",
        background: "#010409",
        color: "#e6edf3",
        fontFamily: "'JetBrains Mono', 'SF Mono', 'Fira Code', monospace",
      }}
    >
      <link
        href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@300;400;500;600;700&display=swap"
        rel="stylesheet"
      />
      <div style={{ padding: "20px 24px 0" }}>
        <div style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 4 }}>
          <h1 style={{ fontSize: 18, fontWeight: 700, color: "#e6edf3", margin: 0 }}>
            Mobile Check Deposit System
          </h1>
          <span style={{ fontSize: 10, color: "#58a6ff", background: "#58a6ff15", padding: "2px 8px", borderRadius: 10 }}>
            SYSTEM FLOW
          </span>
        </div>
        <p style={{ fontSize: 11, color: "#8b949e", margin: "4px 0 16px", maxWidth: 700 }}>
          Interactive flow diagrams — click any node for details. Scroll to zoom, drag to pan. All error paths loop back.
        </p>

        <div
          style={{
            display: "flex",
            gap: 2,
            background: "#0d1117",
            borderRadius: 8,
            padding: 3,
            marginBottom: 16,
            overflowX: "auto",
          }}
        >
          {ROLES.map((r) => (
            <button
              key={r.id}
              onClick={() => { setActiveRole(r.id); setActiveNode(null); }}
              style={{
                padding: "8px 14px",
                borderRadius: 6,
                border: "none",
                background: activeRole === r.id ? "#161b22" : "transparent",
                color: activeRole === r.id ? "#e6edf3" : "#8b949e",
                fontSize: 11,
                fontWeight: activeRole === r.id ? 600 : 400,
                cursor: "pointer",
                fontFamily: "inherit",
                whiteSpace: "nowrap",
                transition: "all 0.15s",
                display: "flex",
                alignItems: "center",
                gap: 6,
                boxShadow: activeRole === r.id ? "0 1px 3px rgba(0,0,0,0.4)" : "none",
              }}
            >
              <span style={{ fontSize: 13 }}>{r.icon}</span>
              {r.label}
            </button>
          ))}
        </div>

        <div style={{ marginBottom: 8 }}>
          <h2 style={{ fontSize: 14, fontWeight: 600, color: "#e6edf3", margin: 0 }}>
            {data.title}
          </h2>
          <p style={{ fontSize: 10, color: "#8b949e", margin: "2px 0 0" }}>
            {data.subtitle}
          </p>
        </div>

        <Legend />
      </div>

      <div
        style={{
          margin: "0 24px",
          border: "1px solid #21262d",
          borderRadius: 8,
          height: "calc(100vh - 280px)",
          minHeight: 400,
          overflow: "hidden",
        }}
      >
        <FlowDiagram
          data={data}
          activeNode={activeNode}
          setActiveNode={setActiveNode}
        />
      </div>

      <div style={{ padding: "0 24px 24px" }}>
        {selectedNode ? (
          <NodeDetail node={selectedNode} edges={data.edges} nodeMap={nodeMap} />
        ) : (
          <p style={{ fontSize: 10, color: "#484f58", marginTop: 12, textAlign: "center" }}>
            Click any node in the diagram to view its connections and details
          </p>
        )}
      </div>
    </div>
  );
}
