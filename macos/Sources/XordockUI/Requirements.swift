import SwiftUI
import XordockCore

/// Requirements list with fix buttons (S011 W-04).
public struct RequirementsView: View {
    let result: DoctorResult?
    let checking: Bool
    let onRecheck: () -> Void
    let onClose: () -> Void

    public init(result: DoctorResult?, checking: Bool, onRecheck: @escaping () -> Void, onClose: @escaping () -> Void) {
        self.result = result
        self.checking = checking
        self.onRecheck = onRecheck
        self.onClose = onClose
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack(spacing: 12) {
                Image(systemName: result?.hasErrors == true ? "wrench.and.screwdriver.fill" : "checkmark.seal.fill")
                    .font(.system(size: 30))
                    .foregroundStyle(result?.hasErrors == true ? Color.orange : Color.green)
                VStack(alignment: .leading, spacing: 2) {
                    Text(result?.hasErrors == true ? "Required components are missing" : "Requirements").font(.title2.weight(.semibold))
                    Text(result?.hasErrors == true
                         ? "Without them the vault can't be opened in Finder. Fix them and click \"Check again\"."
                         : "Everything xordock needs.")
                        .foregroundStyle(.secondary)
                }
            }
            VStack(spacing: 0) {
                ForEach(result?.checks ?? []) { c in
                    HStack(alignment: .top, spacing: 12) {
                        Image(systemName: c.levelValue == .neutral ? "questionmark.circle" : c.levelValue.symbol)
                            .foregroundStyle(c.levelValue.color)
                            .frame(width: 20)
                        VStack(alignment: .leading, spacing: 3) {
                            Text(c.title).font(.headline)
                            Text(c.detail).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
                        }
                        Spacer(minLength: 8)
                        if c.level != "ok", let label = c.fixLabel, let s = c.fixURL, let url = URL(string: s) {
                            Link(label, destination: url).buttonStyle(SecondaryButtonStyle())
                        }
                    }
                    .padding(.vertical, 10)
                    Divider()
                }
            }
            HStack {
                if checking { ProgressView().controlSize(.small); Text("Checking…").foregroundStyle(.secondary) }
                Spacer()
                Button("Check again", action: onRecheck).buttonStyle(SecondaryButtonStyle()).disabled(checking)
                Button("Close", action: onClose).buttonStyle(PrimaryButtonStyle()).keyboardShortcut(.defaultAction)
            }
        }
        .padding(24)
        .frame(width: 600)
    }
}
