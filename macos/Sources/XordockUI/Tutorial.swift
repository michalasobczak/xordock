import Observation
import SwiftUI
import XordockCore

// Samouczek (S010): wprowadzenie, dymki przy elementach, pasek kroków,
// pytanie przy starcie. Widoki tylko pokazują; akcje wykonuje użytkownik.

/// Stan samouczka współdzielony przez okno (kotwice dymków, pasek, arkusze).
@MainActor
@Observable
public final class TourController {
    public var tour = Tour()
    public var bubbleVisible = true
    /// Sejf, którego dotyczą warunki kroków (wybrany w oknie).
    public var vault: () -> VaultInfo? = { nil }

    public init() {}

    public var step: TourStep? { tour.step }

    public func start() {
        tour.start(vault())
        bubbleVisible = true
    }
    public func next() { tour.next(vault()); bubbleVisible = true }
    public func back() { tour.back(vault()); bubbleVisible = true }
    public func finish() { tour.finish() }
    /// Wywoływane po każdym odświeżeniu stanu sejfów.
    public func update() { if tour.update(vault()) { bubbleVisible = true } }

    public func isPresented(_ a: TourAnchor) -> Bool {
        a != .none && tour.active && bubbleVisible && tour.step?.anchor == a
    }
}

/// Treść dymka samouczka.
public struct TourBubble: View {
    let step: TourStep
    let number: Int
    let onNext: () -> Void
    let onFinish: () -> Void

    public init(step: TourStep, number: Int, onNext: @escaping () -> Void, onFinish: @escaping () -> Void) {
        self.step = step
        self.number = number
        self.onNext = onNext
        self.onFinish = onFinish
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Krok \(number) z \(Tour.count)").font(.caption).foregroundStyle(.secondary)
            Text(step.title).font(.headline)
            Text(step.text).fixedSize(horizontal: false, vertical: true)
            if step.automatic {
                Label("Przejdę dalej sam, gdy to zrobisz.", systemImage: "hand.point.up.left")
                    .font(.caption).foregroundStyle(.secondary)
            }
            HStack {
                Button("Zakończ samouczek", action: onFinish).buttonStyle(.link)
                Spacer()
                Button(step.automatic ? "Pomiń krok" : (step == .menuBar ? "Gotowe" : "Dalej"), action: onNext)
                    .buttonStyle(step.automatic ? AnyButtonStyle(SecondaryButtonStyle()) : AnyButtonStyle(PrimaryButtonStyle()))
            }
        }
        .padding(16)
        .frame(width: 320)
    }
}

/// Opakowanie stylu przycisku, żeby wybrać styl warunkowo.
public struct AnyButtonStyle: ButtonStyle {
    private let make: (Configuration) -> AnyView
    public init<S: ButtonStyle>(_ s: S) { make = { AnyView(s.makeBody(configuration: $0)) } }
    public func makeBody(configuration: Configuration) -> some View { make(configuration) }
}

struct TourAnchorModifier: ViewModifier {
    let anchor: TourAnchor
    let edge: Edge
    @Environment(TourController.self) private var tc: TourController?

    func body(content: Content) -> some View {
        if let tc {
            content.popover(isPresented: Binding(get: { tc.isPresented(anchor) },
                                                 set: { if !$0 { tc.bubbleVisible = false } }),
                            arrowEdge: edge) {
                if let s = tc.step {
                    TourBubble(step: s, number: tc.tour.number, onNext: tc.next, onFinish: tc.finish)
                }
            }
        } else {
            content
        }
    }
}

public extension View {
    /// Miejsce dymka samouczka dla kroku z tą kotwicą.
    func tourAnchor(_ a: TourAnchor, edge: Edge = .bottom) -> some View {
        modifier(TourAnchorModifier(anchor: a, edge: edge))
    }
}

/// Pasek samouczka na dole okna (S010 T-05).
public struct TourBar: View {
    @Environment(TourController.self) private var tc: TourController?
    public init() {}

    public var body: some View {
        if let tc, let s = tc.step {
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 10) {
                    Image(systemName: "graduationcap").foregroundStyle(Color.accentColor)
                    Text("Samouczek \(tc.tour.number)/\(Tour.count): \(s.title)").font(.callout.weight(.medium))
                    Spacer()
                    if s.anchor != .none && !tc.bubbleVisible {
                        Button("Pokaż dymek") { tc.bubbleVisible = true }
                    }
                    Button("Wstecz", action: tc.back).disabled(tc.tour.number <= 1)
                    Button(s.automatic ? "Pomiń krok" : (s == .menuBar ? "Gotowe" : "Dalej"), action: tc.next)
                    Button("Zakończ", action: tc.finish)
                }
                .buttonStyle(SecondaryButtonStyle())
                if s.anchor == .none {
                    Text(s.text).foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
                }
            }
            .padding(12)
            .background(Color.accentColor.opacity(0.10))
        }
    }
}

/// Wskazówka samouczka wewnątrz arkusza (S010 T-06).
public struct TourHint: View {
    let text: String
    public init(_ text: String) { self.text = text }
    public var body: some View {
        Label(text, systemImage: "graduationcap")
            .padding(10)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Color.accentColor.opacity(0.12), in: RoundedRectangle(cornerRadius: 8))
    }
}

/// Ekrany wprowadzenia (S010 T-03).
public struct IntroView: View {
    @State private var index: Int
    let onFinish: () -> Void
    let onSkip: () -> Void

    public init(start: Int = 0, onFinish: @escaping () -> Void, onSkip: @escaping () -> Void) {
        _index = State(initialValue: start)
        self.onFinish = onFinish
        self.onSkip = onSkip
    }

    public var body: some View {
        let page = Intro.pages[index]
        let last = index == Intro.pages.count - 1
        VStack(spacing: 18) {
            Image(systemName: page.symbol)
                .font(.system(size: 56))
                .foregroundStyle(Color.accentColor)
                .frame(height: 70)
            Text(page.title).font(.title2.weight(.semibold)).multilineTextAlignment(.center)
            let list = page.text.contains("\n")
            Text(page.text)
                .font(.body)
                .lineSpacing(list ? 4 : 1)
                .multilineTextAlignment(list ? .leading : .center)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: 440)
            Spacer(minLength: 0)
            HStack(spacing: 6) {
                ForEach(Intro.pages.indices, id: \.self) { i in
                    Circle().fill(i == index ? Color.accentColor : Color.secondary.opacity(0.3)).frame(width: 7, height: 7)
                }
            }
            HStack {
                Button("Pomiń wprowadzenie", action: onSkip).buttonStyle(.link)
                Spacer()
                if index > 0 {
                    Button("Wstecz") { index -= 1 }.buttonStyle(SecondaryButtonStyle())
                }
                Button(last ? "Przejdź przez aplikację" : "Dalej") {
                    if last { onFinish() } else { index += 1 }
                }
                .buttonStyle(PrimaryButtonStyle())
                .keyboardShortcut(.defaultAction)
            }
        }
        .padding(28)
        .frame(width: 560, height: 420)
    }
}

/// Pytanie przy starcie (S010 T-01).
public struct StartPrompt: View {
    @Binding var askAtLaunch: Bool
    let onStart: () -> Void
    let onLater: () -> Void

    public init(askAtLaunch: Binding<Bool>, onStart: @escaping () -> Void, onLater: @escaping () -> Void) {
        _askAtLaunch = askAtLaunch
        self.onStart = onStart
        self.onLater = onLater
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack(spacing: 14) {
                Image(systemName: "graduationcap.fill").font(.system(size: 36)).foregroundStyle(Color.accentColor)
                VStack(alignment: .leading, spacing: 4) {
                    Text("Uruchomić samouczek?").font(.title2.weight(.semibold))
                    Text("Kilka ekranów o tym, jak xordock chroni pliki, a potem przejście przez aplikację krok po kroku.")
                        .foregroundStyle(.secondary).fixedSize(horizontal: false, vertical: true)
                }
            }
            Toggle("Nie pytaj przy starcie (zmienisz to w Ustawieniach)", isOn: Binding(get: { !askAtLaunch }, set: { askAtLaunch = !$0 }))
            HStack {
                Spacer()
                Button("Nie teraz", action: onLater).buttonStyle(SecondaryButtonStyle()).keyboardShortcut(.cancelAction)
                Button("Uruchom samouczek", action: onStart).buttonStyle(PrimaryButtonStyle()).keyboardShortcut(.defaultAction)
            }
        }
        .padding(24)
        .frame(width: 480)
    }
}
