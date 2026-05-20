import AppKit
import ApplicationServices
import Foundation

struct PasteRequest: Decodable {
    let text: String
    let chord: String
    let delayMs: Int
    let restoreDelayMs: Int

    enum CodingKeys: String, CodingKey {
        case text
        case chord
        case delayMs = "delay_ms"
        case restoreDelayMs = "restore_delay_ms"
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        text = try container.decode(String.self, forKey: .text)
        chord = try container.decode(String.self, forKey: .chord)
        delayMs = try container.decode(Int.self, forKey: .delayMs)
        restoreDelayMs = try container.decodeIfPresent(Int.self, forKey: .restoreDelayMs) ?? 250
    }
}

struct PasteboardSnapshot {
    let items: [[NSPasteboard.PasteboardType: Data]]
}

enum HelperError: LocalizedError {
    case emptyInput
    case accessibilityNotTrusted
    case pasteboardWriteFailed
    case unsupportedChord(String)

    var errorDescription: String? {
        switch self {
        case .emptyInput:
            return "empty helper request"
        case .accessibilityNotTrusted:
            return "accessibility permission is required for VoicePasteHelper"
        case .pasteboardWriteFailed:
            return "failed to write text to the macOS pasteboard"
        case .unsupportedChord(let chord):
            return "unsupported paste chord: \(chord)"
        }
    }
}

@main
struct VoicePasteHelper {
    static func main() {
        do {
            let request = try readRequest()
            try ensureAccessibilityPermission()
            let snapshot = readPasteboard()
            try writePasteboard(request.text)
            sleep(milliseconds: request.delayMs)
            try sendPasteShortcut(chord: request.chord)
            sleep(milliseconds: request.restoreDelayMs)
            restorePasteboard(snapshot)
        } catch {
            FileHandle.standardError.write(Data((error.localizedDescription + "\n").utf8))
            Foundation.exit(1)
        }
    }

    private static func readRequest() throws -> PasteRequest {
        let data = FileHandle.standardInput.readDataToEndOfFile()
        if data.isEmpty {
            throw HelperError.emptyInput
        }
        return try JSONDecoder().decode(PasteRequest.self, from: data)
    }

    private static func ensureAccessibilityPermission() throws {
        let options = [
            kAXTrustedCheckOptionPrompt.takeUnretainedValue() as String: true
        ] as CFDictionary

        if !AXIsProcessTrustedWithOptions(options) {
            throw HelperError.accessibilityNotTrusted
        }
    }

    private static func writePasteboard(_ text: String) throws {
        let pasteboard = NSPasteboard.general
        pasteboard.clearContents()

        if !pasteboard.setString(text, forType: .string) {
            throw HelperError.pasteboardWriteFailed
        }
    }

    private static func readPasteboard() -> PasteboardSnapshot {
        let pasteboard = NSPasteboard.general
        var items: [[NSPasteboard.PasteboardType: Data]] = []

        for item in pasteboard.pasteboardItems ?? [] {
            var values: [NSPasteboard.PasteboardType: Data] = [:]

            for type in item.types {
                if let data = item.data(forType: type) {
                    values[type] = data
                }
            }

            items.append(values)
        }

        return PasteboardSnapshot(items: items)
    }

    private static func restorePasteboard(_ snapshot: PasteboardSnapshot) {
        let pasteboard = NSPasteboard.general
        pasteboard.clearContents()

        if snapshot.items.isEmpty {
            return
        }

        let restoredItems = snapshot.items.map { values in
            let item = NSPasteboardItem()
            for (type, data) in values {
                item.setData(data, forType: type)
            }
            return item
        }

        pasteboard.writeObjects(restoredItems)
    }

    private static func sendPasteShortcut(chord: String) throws {
        switch chord {
        case "ctrl_v":
            postShortcut(modifierKeyCodes: [55], finalFlags: [.maskCommand])
        case "ctrl_shift_v":
            postShortcut(modifierKeyCodes: [55, 56], finalFlags: [.maskCommand, .maskShift])
        default:
            throw HelperError.unsupportedChord(chord)
        }
    }

    private static func postShortcut(modifierKeyCodes: [CGKeyCode], finalFlags: CGEventFlags) {
        let source = CGEventSource(stateID: .hidSystemState)

        let keyCodeV = CGKeyCode(9)
        let eventTap = CGEventTapLocation.cghidEventTap

        for keyCode in modifierKeyCodes {
            let event = CGEvent(keyboardEventSource: source, virtualKey: keyCode, keyDown: true)
            event?.post(tap: eventTap)
        }

        let keyDown = CGEvent(keyboardEventSource: source, virtualKey: keyCodeV, keyDown: true)
        let keyUp = CGEvent(keyboardEventSource: source, virtualKey: keyCodeV, keyDown: false)
        keyDown?.flags = finalFlags
        keyUp?.flags = finalFlags
        keyDown?.post(tap: eventTap)
        keyUp?.post(tap: eventTap)

        for keyCode in modifierKeyCodes.reversed() {
            let event = CGEvent(keyboardEventSource: source, virtualKey: keyCode, keyDown: false)
            event?.post(tap: eventTap)
        }
    }

    private static func sleep(milliseconds: Int) {
        if milliseconds <= 0 {
            return
        }
        usleep(useconds_t(milliseconds * 1_000))
    }
}
